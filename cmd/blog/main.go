package main

import (
	"codnect.io/chrono"
	"context"
	"gitblog/assets"
	"github.com/speps/go-hashids/v2"
	"log"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"text/template"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "index":
			err := runIndex(logger)
			if err != nil {
				trace := string(debug.Stack())
				logger.Error(err.Error(), "trace", trace)
				os.Exit(1)
			}
			return
		}
	}

	err := runServer(logger)
	if err != nil {
		trace := string(debug.Stack())
		logger.Error(err.Error(), "trace", trace)
		os.Exit(1)
	}
}

type application struct {
	config            Config
	logger            *slog.Logger
	mailer            *Mailer
	taskScheduler     chrono.TaskScheduler
	gitHubCodeService *GitHubCodeService
	markdownService   *MarkdownService
	searchService     *SearchService
	hashId            *hashids.HashID
	wg                sync.WaitGroup
	templates         map[string]*template.Template
}

func runServer(logger *slog.Logger) error {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("reading config failed %v\n", err)
	}

	mailer, err := NewMailer(cfg.Smtp.Host,
		cfg.Smtp.Port,
		cfg.Smtp.Username,
		cfg.Smtp.Password,
		cfg.Smtp.Sender)
	if err != nil {
		return err
	}

	searchService, err := NewSearchService(cfg)
	if err != nil {
		return err
	}

	hd := hashids.NewData()
	hd.Salt = cfg.Blog.Secret
	hi, err := hashids.NewWithData(hd)
	if err != nil {
		return err
	}

	templates := make(map[string]*template.Template)
	templates["feedback"] = template.Must(template.ParseFS(assets.EmbeddedHtml, "html/feedback.tmpl"))
	templates["feedback_ok"] = template.Must(template.ParseFS(assets.EmbeddedHtml, "html/feedback_ok.tmpl"))
	templates["index"] = template.Must(template.ParseFS(assets.EmbeddedHtml, "html/index.tmpl"))

	app := &application{
		config:            cfg,
		logger:            logger,
		mailer:            mailer,
		gitHubCodeService: NewGitHubCodeService(),
		markdownService:   NewMarkdownService(),
		searchService:     searchService,
		hashId:            hi,
		taskScheduler:     chrono.NewDefaultTaskScheduler(),
		templates:         templates,
	}

	_, err = app.taskScheduler.ScheduleWithCron(func(ctx context.Context) {
		app.checkBrokenLinks()
	}, "0 0 2 2 * *")
	if err != nil {
		return err
	}

	err = app.updatePosts()
	if err != nil {
		return err
	}

	return app.serveHTTP()
}

func runIndex(logger *slog.Logger) error {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("reading config failed %v\n", err)
	}

	searchService, err := NewSearchService(cfg)
	if err != nil {
		return err
	}

	app := &application{
		config:        cfg,
		logger:        logger,
		searchService: searchService,
	}

	err = app.indexAllPosts()
	if err != nil {
		return err
	}

	return nil
}

func (app *application) indexAllPosts() error {
	app.logger.Info("deleting all documents from search index")
	err := app.searchService.DeleteAll()
	if err != nil {
		return err
	}

	app.logger.Info("reading all posts metadata from files")
	postMetadatas, err := app.readAllMetadata()
	if err != nil {
		return err
	}

	app.logger.Info("indexing posts in search index", slog.Group("posts", "count", len(postMetadatas)))
	err = app.searchService.IndexPosts(postMetadatas)
	if err != nil {
		return err
	}

	app.logger.Info("successfully indexed all posts")

	return nil
}
