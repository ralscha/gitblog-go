package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/meilisearch/meilisearch-go"
)

// Exercise the real Meilisearch client, including asynchronous task failures.
type searchTestServer struct {
	mu        sync.Mutex
	documents map[string][]Document
	fail      string
	tasks     []string
	swaps     int
}

func newSearchTestServer(t *testing.T) (*SearchService, *searchTestServer) {
	t.Helper()
	state := &searchTestServer{documents: map[string][]Document{IndexName: {{Title: "Old", PublishedYear: 2025}}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method == http.MethodGet && parts[0] == "tasks" {
			var id int
			_, _ = fmt.Sscanf(parts[1], "%d", &id)
			_ = json.NewEncoder(w).Encode(map[string]any{"uid": id, "status": state.tasks[id], "error": map[string]string{"message": "rejected by test server", "code": "test_failure"}})
			return
		}
		if r.Method == http.MethodGet && len(parts) == 3 && parts[2] == "settings" {
			_, _ = io.WriteString(w, `{"filterableAttributes":["publishedYear","tags"],"searchableAttributes":["title","body"]}`)
			return
		}
		status := "succeeded"
		operation := ""
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/indexes":
			operation = "create"
			var config meilisearch.IndexConfig
			_ = json.NewDecoder(r.Body).Decode(&config)
			state.documents[config.Uid] = nil
		case r.Method == http.MethodPatch && len(parts) == 3 && parts[2] == "settings":
			operation = "settings"
			var settings meilisearch.Settings
			if err := json.NewDecoder(r.Body).Decode(&settings); err != nil || len(settings.SearchableAttributes) != 2 {
				t.Errorf("replacement lost existing search settings: %v, %+v", err, settings)
			}
		case r.Method == http.MethodPost && len(parts) == 3 && parts[2] == "documents":
			operation = "documents"
			if state.fail != operation {
				var docs []Document
				_ = json.NewDecoder(r.Body).Decode(&docs)
				state.documents[parts[1]] = docs
			}
		case r.Method == http.MethodPost && r.URL.Path == "/swap-indexes":
			operation = "swap"
			if state.fail != operation {
				var swaps []meilisearch.SwapIndexesParams
				_ = json.NewDecoder(r.Body).Decode(&swaps)
				a, b := swaps[0].Indexes[0], swaps[0].Indexes[1]
				state.documents[a], state.documents[b] = state.documents[b], state.documents[a]
				state.swaps++
			}
		case r.Method == http.MethodDelete && len(parts) == 2 && parts[0] == "indexes":
			operation = "delete"
			delete(state.documents, parts[1])
		default:
			t.Errorf("unexpected search request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		if operation == state.fail {
			status = "failed"
		}
		id := len(state.tasks)
		state.tasks = append(state.tasks, status)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"taskUid": id, "status": "enqueued"})
	}))
	t.Cleanup(server.Close)
	return &SearchService{client: meilisearch.New(server.URL), publishedYears: []int{2025}}, state
}

func TestIndexPostsKeepsLiveIndexWhenReplacementFails(t *testing.T) {
	for _, operation := range []string{"create", "settings", "documents", "swap"} {
		t.Run(operation, func(t *testing.T) {
			service, state := newSearchTestServer(t)
			state.fail = operation
			err := service.IndexPosts([]PostMetadata{{Title: "New", Published: "2026-09-01T00:00:00Z"}})
			if err == nil || !strings.Contains(err.Error(), "test_failure") {
				t.Fatalf("expected task failure, got %v", err)
			}
			state.mu.Lock()
			defer state.mu.Unlock()
			if state.swaps != 0 || len(state.documents) != 1 || state.documents[IndexName][0].Title != "Old" {
				t.Fatalf("live index changed or temporary index leaked: %+v", state)
			}
			if years := service.PublishedYears(); len(years) != 1 || years[0] != 2025 {
				t.Fatalf("year navigation changed after failure: %v", years)
			}
		})
	}
}

func TestIndexPostsReplacesAndClearsAllPosts(t *testing.T) {
	service, state := newSearchTestServer(t)
	for _, posts := range [][]PostMetadata{{{Title: "New", Published: "2026-09-01T00:00:00Z"}}, nil} {
		if err := service.IndexPosts(posts); err != nil {
			t.Fatal(err)
		}
		state.mu.Lock()
		if len(state.documents) != 1 || len(state.documents[IndexName]) != len(posts) {
			t.Errorf("unexpected replacement: %v", state.documents)
		}
		state.mu.Unlock()
		if len(service.PublishedYears()) != len(posts) {
			t.Fatalf("stale year navigation: %v", service.PublishedYears())
		}
	}
}

func newPostsRepository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "post.md"), "---\ntitle: New\npublished: 2026-09-01T00:00:00Z\n---\nBody\n")
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("post.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("Add post", &git.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestUpdatePostsRetriesPublicationWithUnchangedMarkdown(t *testing.T) {
	service, state := newSearchTestServer(t)
	templates, err := loadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	app := &application{searchService: service, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), templates: templates, markdownService: NewMarkdownService(), gitHubCodeService: NewGitHubCodeService()}
	app.config.Github.URL = newPostsRepository(t)
	app.config.Blog.PostDir = t.TempDir() // Fresh installations may precreate this directory.
	app.config.Blog.URL = "https://example.com/"
	state.fail = "documents"
	if err := app.updatePosts(); err == nil {
		t.Fatal("expected publication failure")
	}
	path := filepath.Join(app.config.Blog.PostDir, "post.html")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	state.fail = ""
	state.mu.Unlock()
	if err := app.updatePosts(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("unchanged post was needlessly converted")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.swaps != 1 || state.documents[IndexName][0].Title != "New" {
		t.Fatal("publication was not retried")
	}
}

func TestCleanupPreservesTrackedHTML(t *testing.T) {
	dir := newPostsRepository(t)
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"demo/index.html", "demo/index.html.gz"} {
		writeTestFile(t, filepath.Join(dir, path), "authored content")
		if _, err := worktree.Add(path); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(dir, "deleted.html"), "generated orphan")
	app := &application{}
	app.config.Blog.PostDir = dir
	if _, err := app.cleanup(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"demo/index.html", "demo/index.html.gz"} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Fatalf("authored content removed: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "deleted.html")); !os.IsNotExist(err) {
		t.Fatal("orphan was not removed")
	}
}
