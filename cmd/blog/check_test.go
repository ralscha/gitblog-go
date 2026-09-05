package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseIgnoredURLs(t *testing.T) {
	content := "\n# comment\n HTTPS://EXAMPLE.COM/path \r\n\thttps://other.example\t\n"
	want := []string{"https://example.com/path", "https://other.example"}
	if got := parseIgnoredURLs(content); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIgnoredURLs() = %#v, want %#v", got, want)
	}
}

func TestParseIgnoredURLsEmptyLinesDoNotMatchEverything(t *testing.T) {
	if got := parseIgnoredURLs("\n\r\n"); len(got) != 0 {
		t.Fatalf("parseIgnoredURLs() = %#v, want empty", got)
	}
}

func TestCollectLinks(t *testing.T) {
	links, err := collectLinks(`<main><a href="https://example.com/a#part">one</a><a href="/local">two</a></main>`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://example.com/a#part", "/local"}
	if !reflect.DeepEqual(links, want) {
		t.Fatalf("collectLinks() = %#v, want %#v", links, want)
	}
	if got := removeFragment(links[0]); got != "https://example.com/a" {
		t.Fatalf("removeFragment() = %q", got)
	}
}

func TestCheckBrokenLinksReportsRedirectsWithoutIgnoreFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://example.com/new")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer server.Close()

	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "post.md"), "---\ntitle: Post\npublished: 2026-09-05T12:00:00Z\n---\nBody\n")
	writeTestFile(t, filepath.Join(dir, "post.html"), `<a href="`+server.URL+`/old">old</a>`)
	app := &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	app.config.Blog.PostDir = dir

	if err := app.checkBrokenLinks(); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(dir, "report", "urlcheck.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"301", "https://example.com/new"} {
		if !strings.Contains(string(report), value) {
			t.Fatalf("report does not contain %q: %s", value, report)
		}
	}
}
