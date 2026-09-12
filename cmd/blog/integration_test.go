package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Point this only at a disposable Meilisearch instance: it replaces the posts index.
func TestMeilisearchIntegration(t *testing.T) {
	host := os.Getenv("GITBLOG_TEST_MEILI_URL")
	if host == "" {
		t.Skip("GITBLOG_TEST_MEILI_URL is not set")
	}
	var cfg Config
	cfg.Meilisearch.Host = host
	cfg.Meilisearch.Key = os.Getenv("GITBLOG_TEST_MEILI_KEY")
	cfg.Blog.Title = "Integration blog"
	service, err := NewSearchService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	posts := []PostMetadata{
		{Title: "Go integration", Summary: "A searchable summary", Markdown: "needle", URL: "go.html", Published: "2026-09-01T00:00:00Z", Tags: []string{"go"}},
		{Title: "Older", URL: "old.html", Published: "2025-01-01T00:00:00Z", Tags: []string{"other"}},
	}
	if err := service.IndexPosts(posts); err != nil {
		t.Fatal(err)
	}
	for name, query := range map[string]func() ([]PostMetadata, error){
		"year": func() ([]PostMetadata, error) { return service.SearchPostsOfYear(2026) },
		"tag":  func() ([]PostMetadata, error) { return service.SearchWithTag("go") },
		"text": func() ([]PostMetadata, error) { return service.Search("needle") },
	} {
		t.Run(name, func(t *testing.T) {
			result, err := query()
			if err != nil || len(result) != 1 || result[0].Title != "Go integration" {
				t.Fatalf("search = %+v, %v", result, err)
			}
		})
	}
	// A restart must reload year navigation from the existing database.
	service, err = NewSearchService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if years := service.PublishedYears(); len(years) != 2 || years[0] != 2026 || years[1] != 2025 {
		t.Fatalf("years = %v", years)
	}
	templates, err := loadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	app := &application{config: cfg, searchService: service, templates: templates, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, path := range []string{"/", "/index.html?year=2026", "/index.html?tag=go", "/index.html?query=needle"} {
		rr := httptest.NewRecorder()
		app.routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Go integration") {
			t.Fatalf("%s = %d, %s", path, rr.Code, rr.Body.String())
		}
	}
	if err := service.IndexPosts(nil); err != nil {
		t.Fatal(err)
	}
	if result, err := service.Search(""); err != nil || len(result) != 0 {
		t.Fatalf("empty index = %+v, %v", result, err)
	}
	if len(service.PublishedYears()) != 0 {
		t.Fatal("empty index retained years")
	}
}

func TestMermaidIntegration(t *testing.T) {
	if os.Getenv("GITBLOG_TEST_MERMAID") != "1" {
		t.Skip("GITBLOG_TEST_MERMAID is not enabled")
	}
	result, err := NewMarkdownService().Convert("```mermaid\ngraph TD\n A[Blog] --> B[Reader]\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "<svg") || !strings.Contains(result, "Reader") {
		t.Fatalf("diagram did not render: %s", result)
	}
}

func TestSMTPIntegration(t *testing.T) {
	address := os.Getenv("GITBLOG_TEST_SMTP_ADDR")
	if address == "" {
		t.Skip("GITBLOG_TEST_SMTP_ADDR is not set")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	mailer, err := NewMailer(host, port, "", "", "review@example.com", "none")
	if err != nil {
		t.Fatal(err)
	}
	if err := mailer.SendFeedback("reader@example.com", "post.html", "Integration feedback"); err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(os.Getenv("GITBLOG_TEST_INBUCKET_URL") + "/api/v1/mailbox/review")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var messages []struct {
		Subject string `json:"subject"`
	}
	if err := json.NewDecoder(response.Body).Decode(&messages); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(messages) == 0 || messages[len(messages)-1].Subject != "Feedback: post.html" {
		t.Fatalf("feedback not delivered: %d, %+v", response.StatusCode, messages)
	}
}
