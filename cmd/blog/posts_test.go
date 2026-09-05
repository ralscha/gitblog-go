package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadAllMetadataSkipsDraftsAndParsesPublishedTime(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "published.md"), `---
title: Published
published: 2026-09-05T12:00:00Z
tags: [go]
---
Body
`)
	writeTestFile(t, filepath.Join(dir, "draft.md"), `---
title: Draft
published: 2026-09-05T12:00:00Z
draft: true
---
Hidden
`)

	app := &application{}
	app.config.Blog.PostDir = dir
	posts, err := app.readAllMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || posts[0].Title != "Published" || posts[0].Markdown != "Body\n" {
		t.Fatalf("unexpected posts: %#v", posts)
	}
	wantTime := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	if !posts[0].PublishedTS.Equal(wantTime) {
		t.Fatalf("PublishedTS = %v, want %v", posts[0].PublishedTS, wantTime)
	}
}

func TestReadAllMetadataReportsFileForInvalidDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.md")
	writeTestFile(t, path, "---\ntitle: Broken\npublished: yesterday\n---\nBody\n")
	app := &application{}
	app.config.Blog.PostDir = dir
	if _, err := app.readAllMetadata(); err == nil {
		t.Fatal("readAllMetadata() accepted an invalid date")
	}
}

func TestValidatePostHeader(t *testing.T) {
	tests := []PostHeader{
		{Published: "2026-09-05T12:00:00Z"},
		{Title: "Post", Published: "2026-09-05T12:00:00Z", Updated: "2025-09-05T12:00:00Z"},
	}
	for _, header := range tests {
		if _, _, err := validatePostHeader(header, "post.md"); err == nil {
			t.Fatalf("validatePostHeader() accepted %#v", header)
		}
	}
}

func TestCleanupRemovesOrphansAndDraftsButPreservesReports(t *testing.T) {
	dir := t.TempDir()
	orphanHTML := filepath.Join(dir, "orphan.html")
	compressedOnlyHTML := filepath.Join(dir, "compressed-only.html")
	draftHTML := filepath.Join(dir, "draft.html")
	scratchHTML := filepath.Join(dir, "scratch", "DRAFT.html")
	reportHTML := filepath.Join(dir, "report", "urlcheck.html")
	writeTestFile(t, orphanHTML, "orphan")
	writeTestFile(t, orphanHTML+".gz", "gzip")
	writeTestFile(t, orphanHTML+".br", "brotli")
	writeTestFile(t, compressedOnlyHTML+".gz", "gzip")
	writeTestFile(t, filepath.Join(dir, "draft.md"), "---\ntitle: Draft\npublished: 2026-09-05T12:00:00Z\ndraft: true\n---\nBody\n")
	writeTestFile(t, draftHTML, "draft")
	writeTestFile(t, filepath.Join(dir, "scratch", "DRAFT.md"), "unfinished scratch post")
	writeTestFile(t, scratchHTML, "scratch")
	writeTestFile(t, reportHTML, "report")

	app := &application{}
	app.config.Blog.PostDir = dir
	changed, err := app.cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("cleanup() reported no changes")
	}
	for _, path := range []string{orphanHTML, orphanHTML + ".gz", orphanHTML + ".br", compressedOnlyHTML + ".gz", draftHTML, scratchHTML} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("generated file still exists: %s", path)
		}
	}
	if _, err := os.Stat(reportHTML); err != nil {
		t.Fatalf("report was removed: %v", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
