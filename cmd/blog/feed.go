package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/gorilla/feeds"
	"github.com/sabloger/sitemap-generator/smg"
)

func (app *application) writeFeeds(postMetadata []PostMetadata) error {
	posts := slices.Clone(postMetadata)
	for i := range posts {
		published, err := time.Parse(time.RFC3339, posts[i].Published)
		if err != nil {
			return fmt.Errorf("parse published time for %s: %w", posts[i].URL, err)
		}
		posts[i].PublishedTS = published
	}
	slices.SortFunc(posts, func(a, b PostMetadata) int {
		return b.PublishedTS.Compare(a.PublishedTS)
	})

	var lastUpdated time.Time
	feedItems := make([]*feeds.Item, len(posts))
	for i, post := range posts {
		description := ""
		if post.Summary != "" {
			description = post.Summary
		} else {
			description = post.Title
		}

		published, err := time.Parse(time.RFC3339, post.Published)
		if err != nil {
			return fmt.Errorf("parse published time for %s: %w", post.URL, err)
		}

		updated := published
		if post.Updated != "" {
			updated, err = time.Parse(time.RFC3339, post.Updated)
			if err != nil {
				return fmt.Errorf("parse updated time for %s: %w", post.URL, err)
			}
		}
		if updated.After(lastUpdated) {
			lastUpdated = updated
		}

		feedItems[i] = &feeds.Item{
			Title:       post.Title,
			Link:        &feeds.Link{Href: absoluteBlogURL(app.config.Blog.URL, post.URL)},
			Source:      &feeds.Link{Href: absoluteBlogURL(app.config.Blog.URL, post.URL)},
			Author:      &feeds.Author{Name: app.config.Blog.Author},
			Description: description,
			Id:          absoluteBlogURL(app.config.Blog.URL, post.URL),
			Updated:     updated,
			Created:     published,
		}
	}
	if lastUpdated.IsZero() {
		lastUpdated = time.Now()
	}

	feed := &feeds.Feed{
		Title:       app.config.Blog.Title,
		Link:        &feeds.Link{Href: app.config.Blog.URL},
		Description: app.config.Blog.Description,
		Author:      &feeds.Author{Name: app.config.Blog.Author},
		Created:     lastUpdated,
		Updated:     lastUpdated,
		Items:       feedItems,
	}

	atom, err := feed.ToAtom()
	if err != nil {
		return fmt.Errorf("failed to generate atom feed: %w", err)
	}

	rss, err := feed.ToRss()
	if err != nil {
		return fmt.Errorf("failed to generate rss feed: %w", err)
	}

	json, err := feed.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to generate json feed: %w", err)
	}

	workDir := app.config.Blog.PostDir

	feedFiles := map[string]string{
		"feed.atom": atom,
		"feed.rss":  rss,
		"feed.json": json,
	}
	for name, content := range feedFiles {
		if err := writeFileAtomic(filepath.Join(workDir, name), []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	for name := range feedFiles {
		err = compressFileWithGzip(filepath.Join(workDir, name))
		if err != nil {
			return fmt.Errorf("failed to gzip: %w", err)
		}

		err = compressFileWithBrotli(filepath.Join(workDir, name))
		if err != nil {
			return fmt.Errorf("failed to brotli: %w", err)
		}
	}

	return nil
}

func (app *application) writeSitemap(postMetadata []PostMetadata) error {
	var lastUpdated time.Time
	for _, post := range postMetadata {
		updated, err := postLastModified(post)
		if err != nil {
			return err
		}

		if updated.After(lastUpdated) {
			lastUpdated = updated.Truncate(time.Second)
		}
	}
	if lastUpdated.IsZero() {
		lastUpdated = time.Now().Truncate(time.Second)
	}

	sm := smg.NewSitemap(true)
	sm.SetName("sitemap")
	sm.SetHostname(app.config.Blog.URL)
	sm.SetOutputPath(app.config.Blog.PostDir)
	sm.SetLastMod(&lastUpdated)
	sm.SetCompress(false)

	for _, post := range postMetadata {
		updated, err := postLastModified(post)
		if err != nil {
			return err
		}
		updated = updated.Truncate(time.Second)

		err = sm.Add(&smg.SitemapLoc{
			Loc:        post.URL,
			LastMod:    &updated,
			ChangeFreq: smg.Yearly,
			Priority:   0.7,
		})
		if err != nil {
			return fmt.Errorf("failed to add sitemap loc: %w", err)
		}
	}

	_, err := sm.Save()
	if err != nil {
		return fmt.Errorf("failed to save sitemap: %w", err)
	}

	// compress sitemap
	err = compressFileWithGzip(filepath.Join(app.config.Blog.PostDir, "sitemap.xml"))
	if err != nil {
		return fmt.Errorf("failed to gzip: %w", err)
	}

	err = compressFileWithBrotli(filepath.Join(app.config.Blog.PostDir, "sitemap.xml"))
	if err != nil {
		return fmt.Errorf("failed to brotli: %w", err)
	}

	return nil
}

func postLastModified(post PostMetadata) (time.Time, error) {
	value := post.Published
	field := "published"
	if post.Updated != "" {
		value = post.Updated
		field = "updated"
	}
	timestamp, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s time for %s: %w", field, post.URL, err)
	}
	return timestamp, nil
}
