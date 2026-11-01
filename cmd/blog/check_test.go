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
	"sync"
	"testing"
	"time"
)

func TestCheckLinkDoesNotWaitForResponseBody(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusMovedPermanently, http.StatusTooManyRequests, http.StatusSwitchingProtocols} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if status == http.StatusSwitchingProtocols {
					w.Header().Set("Connection", "Upgrade")
					w.Header().Set("Upgrade", "websocket")
				}
				w.Header().Set("Location", "https://example.com/new")
				w.WriteHeader(status)
				w.(http.Flusher).Flush()
				<-release
			}))
			defer server.Close()
			defer close(release)

			client := server.Client()
			client.Timeout = 5 * time.Second
			if status == http.StatusSwitchingProtocols {
				// Reading an upgraded connection can outlive Client.Timeout.
				client.Timeout = 100 * time.Millisecond
			}
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			done := make(chan URLCheck, 1)
			go func() {
				done <- checkLink(client, URLCheck{URL: server.URL}, make(map[string]string))
			}()

			select {
			case result := <-done:
				if result.Error != "" {
					t.Fatalf("checkLink() error = %s", result.Error)
				}
				wantStatus := status
				if status == http.StatusOK || status == http.StatusTooManyRequests {
					wantStatus = 0
				}
				if result.Status != wantStatus {
					t.Fatalf("checkLink() status = %d, want %d", result.Status, wantStatus)
				}
				if status == http.StatusMovedPermanently && result.Location != "https://example.com/new" {
					t.Fatalf("checkLink() location = %q", result.Location)
				}
			case <-time.After(time.Second):
				t.Fatal("link checker waited for the response body after receiving headers")
			}
		})
	}
}

func TestCheckLinkTimesOutWaitingForHeadersAndContinues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-r.Context().Done()
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 100 * time.Millisecond
	failedDomains := make(map[string]string)
	result := checkLink(client, URLCheck{URL: server.URL + "/slow"}, failedDomains)
	if !strings.Contains(result.Error, "Client.Timeout") {
		t.Fatalf("checkLink() error = %q, want client timeout", result.Error)
	}
	result = checkLink(client, URLCheck{URL: server.URL + "/healthy"}, failedDomains)
	if result.Error != "" || result.Status != 0 {
		t.Fatalf("healthy link after timeout was reported as broken: %+v", result)
	}
}

type linkCheckLogWriter chan string

func (logs linkCheckLogWriter) Write(p []byte) (int, error) {
	logs <- string(p)
	return len(p), nil
}

func TestCheckBrokenLinksLogsFailuresBeforeFinishing(t *testing.T) {
	nextStarted := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/broken" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		close(nextStarted)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	defer unblock()

	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "post.md"), "---\ntitle: Post\npublished: 2026-09-05T12:00:00Z\n---\nBody\n")
	writeTestFile(t, filepath.Join(dir, "post.html"), `<a href="`+server.URL+`/broken">broken</a><a href="`+server.URL+`/next">next</a>`)
	logs := make(linkCheckLogWriter, 32)
	app := &application{logger: slog.New(slog.NewJSONHandler(logs, nil))}
	app.config.Blog.PostDir = dir

	var checkErr error
	done := make(chan struct{})
	go func() {
		checkErr = app.checkBrokenLinks()
		close(done)
	}()
	defer func() {
		unblock()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("link checker did not finish after releasing the server")
		}
	}()

	select {
	case <-nextStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("link checker did not reach the second link")
	}
	var output strings.Builder
	for len(logs) > 0 {
		output.WriteString(<-logs)
	}
	if !strings.Contains(output.String(), `"msg":"broken link"`) || !strings.Contains(output.String(), server.URL+"/broken") {
		t.Fatalf("failure was not logged before checking the next link: %s", output.String())
	}
	if strings.Contains(output.String(), `"checked":2`) {
		t.Fatalf("progress counted an unfinished request: %s", output.String())
	}

	unblock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("link checker did not finish")
	}
	if checkErr != nil {
		t.Fatal(checkErr)
	}
	for len(logs) > 0 {
		output.WriteString(<-logs)
	}
	for _, want := range []string{`"percent":100`, `"msg":"link-check completed"`, `"broken":1`} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("logs do not contain %s: %s", want, output.String())
		}
	}
}

func TestCheckBrokenLinksReportsConnectionFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "post.md"), "---\ntitle: Post\npublished: 2026-09-05T12:00:00Z\n---\nBody\n")
	writeTestFile(t, filepath.Join(dir, "post.html"), `<a href="`+server.URL+`/unreachable">broken</a>`)
	app := &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	app.config.Blog.PostDir = dir
	if err := app.checkBrokenLinks(); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(dir, "report", "urlcheck.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(report), server.URL+"/unreachable") {
		t.Fatalf("report omitted connection failure: %s", report)
	}
}

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
