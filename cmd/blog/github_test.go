package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patrickmn/go-cache"
)

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
