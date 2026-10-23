package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patrickmn/go-cache"
	"golang.org/x/net/html"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestInsertCodePreservesRepositoryNameAndCodeFences(t *testing.T) {
	service := NewGitHubCodeService()
	code := "```html\n<script>alert('example')</script>\n```\n"
	service.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != gitHubRawLink+"owner/blob-tools/main/example.md" {
			t.Errorf("unexpected raw URL: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(code))}, nil
	})
	markdown, err := service.InsertCode("[github:https://github.com/owner/blob-tools/blob/main/example.md]")
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := NewMarkdownService().Convert(markdown)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(strings.NewReader(rendered))
	if err != nil {
		t.Fatal(err)
	}
	var codeText string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" {
			t.Error("embedded source escaped its code block")
		}
		if n.Type == html.ElementNode && n.Data == "code" {
			codeText = nodeText(n)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if strings.TrimSpace(codeText) != strings.TrimSpace(code) {
		t.Fatalf("embedded code changed: %q", codeText)
	}
}

func TestInsertCodeRejectsInvalidRanges(t *testing.T) {
	service := NewGitHubCodeService()
	service.siteCache.Set(gitHubRawLink+"owner/repo/main/file.go", "one\ntwo\nthree", cache.DefaultExpiration)
	for _, fragment := range []string{"#L0", "#L0-L2", "#L2-L0", "#L3-L1"} {
		if _, err := service.InsertCode("[github:https://github.com/owner/repo/blob/main/file.go" + fragment + "]"); err == nil {
			t.Errorf("accepted invalid range %s", fragment)
		}
	}
}

func TestFetchCodeRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxGitHubCode+1)))
	}))
	defer server.Close()

	service := &GitHubCodeService{
		httpClient: server.Client(),
		siteCache:  cache.New(cache.NoExpiration, cache.NoExpiration),
	}
	if _, err := service.fetchCode(server.URL); err == nil {
		t.Fatal("fetchCode() accepted an oversized response")
	}
}

func TestFetchCodeCachesSuccessfulResponse(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte("code"))
	}))
	defer server.Close()

	service := &GitHubCodeService{
		httpClient: server.Client(),
		siteCache:  cache.New(cache.NoExpiration, cache.NoExpiration),
	}
	for range 2 {
		code, err := service.fetchCode(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		if code != "code" {
			t.Fatalf("code = %q", code)
		}
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}
