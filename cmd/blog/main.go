package main

import (
	"context"
	"fmt"
	"gitblog/assets"
	"html/template"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"

	"codnect.io/chrono"
	shiki "github.com/ralscha/shiki-go"
	"github.com/speps/go-hashids/v2"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	if err := run(os.Args[1:], logger); err != nil {
		trace := string(debug.Stack())
		logger.Error(err.Error(), "trace", trace)
		os.Exit(1)
	}
}

func run(args []string, logger *slog.Logger) error {
	if len(args) > 1 {
		return fmt.Errorf("unexpected arguments: %q", args[1:])
	}
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "serve":
		return runServer(logger)
	case "index":
		return runIndex(logger)
	case "rebuild":
		return runRebuild(logger)
	case "report":
		return runReport(logger)
	default:
		return fmt.Errorf("unknown command %q (expected serve, index, rebuild, or report)", command)
	}
}

type application struct {
	config            Config
	logger            *slog.Logger
	mailer            *Mailer
	taskScheduler     chrono.TaskScheduler
	gitHubCodeService *GitHubCodeService
	markdownService   *MarkdownService
	highlighter       *shiki.Highlighter
	searchService     *SearchService
	hashID            *hashids.HashID
	wg                sync.WaitGroup
	updateMu          sync.Mutex
	templates         map[string]*template.Template
}

func runServer(logger *slog.Logger) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	mailer, err := NewMailer(cfg.SMTP.Host,
		cfg.SMTP.Port,
		cfg.SMTP.Username,
		cfg.SMTP.Password,
		cfg.SMTP.Sender,
		cfg.SMTP.TLSPolicy)
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

	templates, err := loadTemplates()
	if err != nil {
		return err
	}

	highlighter, err := newHighlighter()
	if err != nil {
		return fmt.Errorf("initialize syntax highlighter: %w", err)
	}
	defer func() { _ = highlighter.Close() }()

	app := &application{
		config:            cfg,
		logger:            logger,
		mailer:            mailer,
		gitHubCodeService: NewGitHubCodeService(),
		markdownService:   NewMarkdownService(),
		highlighter:       highlighter,
		searchService:     searchService,
		hashID:            hi,
		taskScheduler:     chrono.NewDefaultTaskScheduler(),
		templates:         templates,
	}

	_, err = app.taskScheduler.ScheduleWithCron(func(ctx context.Context) {
		if err := app.checkBrokenLinks(); err != nil {
			app.logger.Error("broken-link check failed", "error", err)
		}
	}, "0 40 5 1 * *")
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
		return err
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

func runReport(logger *slog.Logger) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	app := &application{
		config: cfg,
		logger: logger,
	}

	return app.checkBrokenLinks()
}

func runRebuild(logger *slog.Logger) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	searchService, err := NewSearchService(cfg)
	if err != nil {
		return err
	}
	templates, err := loadTemplates()
	if err != nil {
		return err
	}

	highlighter, err := newHighlighter()
	if err != nil {
		return fmt.Errorf("initialize syntax highlighter: %w", err)
	}
	defer func() { _ = highlighter.Close() }()

	app := &application{
		config:            cfg,
		logger:            logger,
		gitHubCodeService: NewGitHubCodeService(),
		markdownService:   NewMarkdownService(),
		highlighter:       highlighter,
		searchService:     searchService,
		templates:         templates,
	}
	return app.rebuildPosts()
}

func loadTemplates() (map[string]*template.Template, error) {
	files := map[string]string{
		"feedback":    "html/feedback.tmpl",
		"feedback_ok": "html/feedback_ok.tmpl",
		"index":       "html/index.tmpl",
		"post":        "html/post.tmpl",
	}
	templates := make(map[string]*template.Template, len(files))
	for name, file := range files {
		parsed, err := template.ParseFS(assets.EmbeddedHTML, file)
		if err != nil {
			return nil, fmt.Errorf("parse %s template: %w", name, err)
		}
		templates[name] = parsed
	}
	return templates, nil
}

func (app *application) indexAllPosts() error {
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
