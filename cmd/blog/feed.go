package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/feeds"
	"github.com/sabloger/sitemap-generator/smg"
)

func (app *application) writeFeeds(postMetadata []PostMetadata) error {

	lastUpdated := time.Now()
	feedItems := make([]*feeds.Item, len(postMetadata))
	for i, post := range postMetadata {
		description := ""
		if post.Summary != "" {
			description = post.Summary
		} else {
			description = post.Title
		}

		published, err := time.Parse(time.RFC3339, post.Published)
		if err != nil {
			return fmt.Errorf("failed to parse published time: %w", err)
		}

		var updated time.Time
		if post.Updated != "" {
			updated, err = time.Parse(time.RFC3339, post.Updated)
			if err != nil {
				return fmt.Errorf("failed to parse updated time: %w", err)
			}

			if updated.After(lastUpdated) {
				lastUpdated = updated
			}
		}

		feedItems[i] = &feeds.Item{
			Title:       post.Title,
			Link:        &feeds.Link{Href: app.config.Blog.URL + post.URL},
			Source:      &feeds.Link{Href: app.config.Blog.URL + post.URL},
			Author:      &feeds.Author{Name: app.config.Blog.Author},
			Description: description,
			Id:          app.config.Blog.URL + post.URL,
			Updated:     updated,
			Created:     published,
		}
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

	err = os.WriteFile(filepath.Join(workDir, "feed.atom"), []byte(atom), 0644)
	if err != nil {
		return fmt.Errorf("failed to write atom feed: %w", err)
	}

	err = os.WriteFile(filepath.Join(workDir, "feed.rss"), []byte(rss), 0644)
	if err != nil {
		return fmt.Errorf("failed to write rss feed: %w", err)
	}

	err = os.WriteFile(filepath.Join(workDir, "feed.json"), []byte(json), 0644)
	if err != nil {
		return fmt.Errorf("failed to write json feed: %w", err)
	}

	for _, f := range []string{"feed.atom", "feed.rss", "feed.json"} {
		err = compressFileWithGzip(filepath.Join(workDir, f))
		if err != nil {
			return fmt.Errorf("failed to gzip: %w", err)
		}

		err = compressFileWithBrotli(filepath.Join(workDir, f))
		if err != nil {
			return fmt.Errorf("failed to brotli: %w", err)
		}
	}

	return nil
}

func (app *application) writeSitemap(postMetadata []PostMetadata) error {
	lastUpdated := time.Now().Truncate(time.Second)
	for _, post := range postMetadata {
		var updated time.Time
		if post.Updated != "" {
			var err error
			updated, err = time.Parse(time.RFC3339, post.Updated)
			if err != nil {
				return fmt.Errorf("failed to parse updated time: %w", err)
			}

			if updated.After(lastUpdated) {
				lastUpdated = updated.Truncate(time.Second)
			}
		}
	}

	sm := smg.NewSitemap(true)
	sm.SetName("sitemap")
	sm.SetHostname(app.config.Blog.URL)
	sm.SetOutputPath(app.config.Blog.PostDir)
	sm.SetLastMod(&lastUpdated)
	sm.SetCompress(false)

	for _, post := range postMetadata {
		var updated time.Time
		if post.Updated != "" {
			var err error
			updated, err = time.Parse(time.RFC3339, post.Updated)
			if err != nil {
				return fmt.Errorf("failed to parse updated time: %w", err)
			}
			updated = updated.Truncate(time.Second)
		}

		err := sm.Add(&smg.SitemapLoc{
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
