package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

func TestNewSearchServiceLoadsAllYearPages(t *testing.T) {
	var offsets []int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/indexes/posts/settings/filterable-attributes":
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"taskUid":1}`)
		case "/tasks/1":
			fmt.Fprint(w, `{"uid":1,"status":"succeeded"}`)
		case "/indexes/posts/documents/fetch":
			var query meilisearch.DocumentsQuery
			if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
				t.Error(err)
				return
			}
			offsets = append(offsets, query.Offset)
			switch query.Offset {
			case 0:
				fmt.Fprint(w, `{"results":[{"publishedYear":2026},{"publishedYear":2026}],"total":3}`)
			case 2:
				fmt.Fprint(w, `{"results":[{"publishedYear":2025}],"total":3}`)
			default:
				t.Errorf("unexpected offset: %d", query.Offset)
				fmt.Fprint(w, `{"results":[],"total":3}`)
			}
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	var cfg Config
	cfg.Meilisearch.Host = server.URL
	service, err := NewSearchService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(service.PublishedYears(), []int{2026, 2025}) || !slices.Equal(offsets, []int64{0, 2}) {
		t.Fatalf("years = %v, offsets = %v", service.PublishedYears(), offsets)
	}
}

func TestTagFilter(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want string
	}{
		{
			name: "plain tag",
			tag:  "selfhost",
			want: `tags = "selfhost"`,
		},
		{
			name: "apostrophe",
			tag:  "selfhost'",
			want: `tags = "selfhost'"`,
		},
		{
			name: "filter syntax",
			tag:  `selfhost" OR publishedYear = 2026`,
			want: `tags = "selfhost\" OR publishedYear = 2026"`,
		},
		{
			name: "backslash",
			tag:  `self\host`,
			want: `tags = "self\\host"`,
		},
		{
			name: "trailing backslash",
			tag:  `selfhost\`,
			want: `tags = "selfhost\\"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tagFilter(tt.tag); got != tt.want {
				t.Errorf("tagFilter(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

func TestMapToPostMetadataReturnsDecodeErrors(t *testing.T) {
	response := &meilisearch.SearchResponse{Hits: meilisearch.Hits{
		meilisearch.Hit{"publishedTs": json.RawMessage(`"not-a-number"`)},
	}}
	if _, err := mapToPostMetadata(response); err == nil {
		t.Fatal("mapToPostMetadata() swallowed a decode error")
	}
}

func TestMapToPostMetadata(t *testing.T) {
	response := &meilisearch.SearchResponse{Hits: meilisearch.Hits{
		meilisearch.Hit{
			"title":       json.RawMessage(`"Post"`),
			"publishedTs": json.RawMessage(`1788609600`),
			"tags":        json.RawMessage(`["go"]`),
		},
	}}
	posts, err := mapToPostMetadata(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || posts[0].Title != "Post" || posts[0].PublishedTS.Unix() != 1788609600 {
		t.Fatalf("unexpected posts: %#v", posts)
	}
}

func TestPublishedYearsReturnsCopy(t *testing.T) {
	service := &SearchService{publishedYears: []int{2026, 2025}}
	years := service.PublishedYears()
	years[0] = 2000
	if service.PublishedYears()[0] != 2026 {
		t.Fatal("PublishedYears() exposed mutable internal state")
	}
}

func TestSortPostsNewest(t *testing.T) {
	posts := []PostMetadata{
		{Title: "old", PublishedTS: time.Unix(1, 0)},
		{Title: "new", PublishedTS: time.Unix(3, 0)},
		{Title: "middle", PublishedTS: time.Unix(2, 0)},
	}
	sortPostsNewest(posts)
	if posts[0].Title != "new" || posts[1].Title != "middle" || posts[2].Title != "old" {
		t.Fatalf("unexpected order: %#v", posts)
	}
}
