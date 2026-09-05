package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteFeedsSortsNewestFirstAndUsesPublishedAsUpdatedFallback(t *testing.T) {
	dir := t.TempDir()
	app := &application{}
	app.config.Blog.PostDir = dir
	app.config.Blog.URL = "https://example.com/"
	app.config.Blog.Title = "Test Blog"
	app.config.Blog.Description = "Tests"
	app.config.Blog.Author = "Author"
	posts := []PostMetadata{
		{Title: "Older", URL: "older.html", Published: "2025-01-02T03:04:05Z"},
		{Title: "Newer", URL: "newer.html", Published: "2026-02-03T04:05:06Z"},
	}

	if err := app.writeFeeds(posts); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "feed.json"))
	if err != nil {
		t.Fatal(err)
	}
	var feed struct {
		Items []struct {
			Title        string `json:"title"`
			DateModified string `json:"date_modified"`
		} `json:"items"`
	}
	if err := json.Unmarshal(content, &feed); err != nil {
		t.Fatal(err)
	}
	if len(feed.Items) != 2 || feed.Items[0].Title != "Newer" || feed.Items[1].Title != "Older" {
		t.Fatalf("feed is not newest-first: %#v", feed.Items)
	}
	if feed.Items[0].DateModified != "2026-02-03T04:05:06Z" {
		t.Fatalf("published date was not used as updated fallback: %#v", feed.Items[0])
	}
	for _, suffix := range []string{".gz", ".br"} {
		if _, err := os.Stat(filepath.Join(dir, "feed.json"+suffix)); err != nil {
			t.Fatalf("missing compressed feed %s: %v", suffix, err)
		}
	}
}

func TestPostLastModified(t *testing.T) {
	post := PostMetadata{
		URL:       "post.html",
		Published: "2025-01-02T03:04:05Z",
		Updated:   "2026-02-03T04:05:06Z",
	}
	got, err := postLastModified(post)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("postLastModified() = %v, want %v", got, want)
	}
}

func TestWriteSitemapUsesPublishedDateWhenUpdatedIsMissing(t *testing.T) {
	dir := t.TempDir()
	app := &application{}
	app.config.Blog.PostDir = dir
	app.config.Blog.URL = "https://example.com/"
	posts := []PostMetadata{{
		URL:       "post.html",
		Published: "2025-01-02T03:04:05Z",
	}}

	if err := app.writeSitemap(posts); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "sitemap.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "2025-01-02") || strings.Contains(string(content), "0001-01-01") {
		t.Fatalf("unexpected sitemap date: %s", content)
	}
}
