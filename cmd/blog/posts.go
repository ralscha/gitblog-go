package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func (app *application) updatePosts() error {
	app.updateMu.Lock()
	defer app.updateMu.Unlock()

	err := app.pullPosts()
	if err != nil {
		return fmt.Errorf("failed to pull posts: %w", err)
	}
	if _, err := app.readAllMetadata(); err != nil {
		return fmt.Errorf("failed to validate posts: %w", err)
	}

	convertChanged, err := app.convertChangedMarkdowns()
	if err != nil {
		return fmt.Errorf("failed to convert markdowns: %w", err)
	}

	cleanupChanged, err := app.cleanup()
	if err != nil {
		return fmt.Errorf("failed to cleanup: %w", err)
	}

	if convertChanged || cleanupChanged {
		return app.publishPosts()
	}

	return nil
}

func (app *application) rebuildPosts() error {
	if _, err := app.readAllMetadata(); err != nil {
		return fmt.Errorf("failed to validate posts: %w", err)
	}
	markdownFiles, err := app.collectAllMarkdownFiles()
	if err != nil {
		return fmt.Errorf("failed to collect markdown files: %w", err)
	}
	for _, markdownFile := range markdownFiles {
		app.logger.Info("rebuilding markdown", "file", markdownFile)
		if err := app.convert(markdownFile); err != nil {
			return fmt.Errorf("failed to convert markdown: %w", err)
		}
	}
	if _, err := app.cleanup(); err != nil {
		return fmt.Errorf("failed to cleanup: %w", err)
	}
	return app.publishPosts()
}

func (app *application) publishPosts() error {
	postMetadata, err := app.readAllMetadata()
	if err != nil {
		return err
	}
	if err := app.writeFeeds(postMetadata); err != nil {
		return err
	}
	if err := app.writeSitemap(postMetadata); err != nil {
		return err
	}
	if err := app.searchService.DeleteAll(); err != nil {
		return err
	}
	return app.searchService.IndexPosts(postMetadata)
}

func (app *application) readAllMetadata() ([]PostMetadata, error) {
	var postMetadatas []PostMetadata

	markdownFiles, err := app.collectAllMarkdownFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to collect markdown files: %w", err)
	}

	for _, markdownFile := range markdownFiles {
		header, body, err := readPostSource(markdownFile)
		if err != nil {
			return nil, err
		}

		// ignore drafts
		if header.Draft {
			continue
		}

		published, _, err := validatePostHeader(header, markdownFile)
		if err != nil {
			return nil, err
		}

		url, err := filepath.Rel(app.config.Blog.PostDir, siblingPath(markdownFile, "html"))
		if err != nil {
			return nil, fmt.Errorf("failed to get relative path: %w", err)
		}

		url = filepath.ToSlash(url)
		feedbackURL := strings.ReplaceAll(url, "/", "-")

		postMetadata := PostMetadata{
			Title:        header.Title,
			URL:          url,
			FeedbackURL:  feedbackURL,
			Published:    header.Published,
			Updated:      header.Updated,
			Summary:      header.Summary,
			Tags:         header.Tags,
			Markdown:     body,
			MarkdownFile: markdownFile,
			Draft:        header.Draft,
			PublishedTS:  published,
		}

		postMetadatas = append(postMetadatas, postMetadata)
	}

	return postMetadatas, nil

}

func readPostSource(markdownFile string) (PostHeader, string, error) {
	var header PostHeader
	content, err := os.ReadFile(markdownFile)
	if err != nil {
		return header, "", fmt.Errorf("read markdown file %s: %w", markdownFile, err)
	}

	matcher := headerPattern.FindStringSubmatch(string(content))
	if matcher == nil {
		return header, "", fmt.Errorf("markdown file %s does not contain valid front matter", markdownFile)
	}

	if err := yaml.Unmarshal([]byte(matcher[1]), &header); err != nil {
		return header, "", fmt.Errorf("unmarshal front matter in %s: %w", markdownFile, err)
	}
	return header, matcher[2], nil
}

func validatePostHeader(header PostHeader, markdownFile string) (time.Time, time.Time, error) {
	if strings.TrimSpace(header.Title) == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("post title is missing in %s", markdownFile)
	}
	published, err := time.Parse(time.RFC3339, header.Published)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse published time in %s: %w", markdownFile, err)
	}

	var updated time.Time
	if header.Updated != "" {
		updated, err = time.Parse(time.RFC3339, header.Updated)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse updated time in %s: %w", markdownFile, err)
		}
		if updated.Before(published) {
			return time.Time{}, time.Time{}, fmt.Errorf("updated time precedes published time in %s", markdownFile)
		}
	}
	return published, updated, nil
}

func (app *application) cleanup() (bool, error) {
	htmlFiles := make(map[string]struct{})

	err := filepath.Walk(app.config.Blog.PostDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking directory: %w", err)
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		if info.IsDir() && path != app.config.Blog.PostDir && info.Name() == "report" {
			return filepath.SkipDir
		}

		if !info.IsDir() {
			switch {
			case isHTMLFile(path):
				htmlFiles[path] = struct{}{}
			case strings.HasSuffix(path, ".html.gz"):
				htmlFiles[strings.TrimSuffix(path, ".gz")] = struct{}{}
			case strings.HasSuffix(path, ".html.br"):
				htmlFiles[strings.TrimSuffix(path, ".br")] = struct{}{}
			}
		}

		return nil
	})

	if err != nil {
		return false, fmt.Errorf("error walking directory: %w", err)
	}

	changed := false
	for htmlFile := range htmlFiles {
		markdownFile := siblingPath(htmlFile, "md")
		_, err := os.Stat(markdownFile)
		if err != nil {
			if !os.IsNotExist(err) {
				return false, fmt.Errorf("failed to get markdown file info: %w", err)
			}

			removed, err := removeGeneratedHTML(htmlFile)
			if err != nil {
				return false, err
			}
			changed = changed || removed
			continue
		}

		if filepath.Base(markdownFile) == "DRAFT.md" {
			removed, err := removeGeneratedHTML(htmlFile)
			if err != nil {
				return false, err
			}
			changed = changed || removed
			continue
		}

		header, _, err := readPostSource(markdownFile)
		if err != nil {
			return false, err
		}
		if header.Draft {
			removed, err := removeGeneratedHTML(htmlFile)
			if err != nil {
				return false, err
			}
			changed = changed || removed
		}
	}
	return changed, nil
}
