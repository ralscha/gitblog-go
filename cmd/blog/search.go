package main

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

const (
	IndexName = "posts"
)

type SearchService struct {
	client         meilisearch.ServiceManager
	yearsMu        sync.RWMutex
	publishedYears []int
}

type Document struct {
	ID            int      `json:"id"`
	Body          string   `json:"body"`
	Summary       string   `json:"summary"`
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	PublishedTs   int64    `json:"publishedTs"`
	UpdatedTs     int64    `json:"updatedTs"`
	PublishedYear int      `json:"publishedYear"`
	Tags          []string `json:"tags"`
}

func NewSearchService(config Config) (*SearchService, error) {
	c := meilisearch.New(config.Meilisearch.Host, meilisearch.WithAPIKey(config.Meilisearch.Key))

	index := c.Index(IndexName)

	filterableAttrs := []any{"publishedYear", "tags"}
	task, err := index.UpdateFilterableAttributes(&filterableAttrs)
	if err != nil {
		return nil, err
	}
	_, err = index.WaitForTask(task.TaskUID, 5*time.Second)
	if err != nil {
		return nil, err
	}

	var publishedYears []int

	request := meilisearch.DocumentsQuery{
		Fields: []string{"publishedYear"},
		Limit:  9_000,
	}
	var response meilisearch.DocumentsResult

	err = index.GetDocuments(&request, &response)
	if err != nil {
		return nil, err
	}

	var years []struct {
		PublishedYear int `json:"publishedYear"`
	}
	err = response.Results.DecodeInto(&years)
	if err != nil {
		return nil, fmt.Errorf("decode published years: %w", err)
	}
	for _, year := range years {
		publishedYears = append(publishedYears, year.PublishedYear)
	}

	publishedYears = unique(publishedYears)
	slices.SortFunc(publishedYears, func(i, j int) int {
		return cmp.Compare(j, i)
	})

	return &SearchService{
		client:         c,
		publishedYears: publishedYears,
	}, nil
}

func (s *SearchService) PublishedYears() []int {
	s.yearsMu.RLock()
	defer s.yearsMu.RUnlock()
	return slices.Clone(s.publishedYears)
}

func (s *SearchService) setPublishedYears(years []int) {
	s.yearsMu.Lock()
	defer s.yearsMu.Unlock()
	s.publishedYears = slices.Clone(years)
}

func unique(intSlice []int) []int {
	seen := make(map[int]struct{}, len(intSlice))
	list := make([]int, 0, len(intSlice))
	for _, entry := range intSlice {
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		list = append(list, entry)
	}
	return list
}

func (s *SearchService) waitForTask(taskUID int64) error {
	_, err := s.client.Index(IndexName).WaitForTask(taskUID, 5*time.Second)
	return err
}

func (s *SearchService) DeleteAll() error {
	task, err := s.client.Index(IndexName).DeleteAllDocuments(nil)
	if err != nil {
		return err
	}
	if err := s.waitForTask(task.TaskUID); err != nil {
		return err
	}

	s.setPublishedYears(nil)

	return nil
}

func (s *SearchService) IndexPosts(posts []PostMetadata) error {
	if len(posts) == 0 {
		s.setPublishedYears(nil)
		return nil
	}

	documents := make([]Document, len(posts))
	publishedYears := make([]int, 0, len(posts))
	for i, post := range posts {
		publishedTime, err := time.Parse(time.RFC3339, post.Published)
		if err != nil {
			return err
		}
		publishedYear := publishedTime.Year()
		publishedYears = append(publishedYears, publishedYear)

		var updatedSeconds int64 = 0
		if post.Updated != "" {
			updatedTime, err := time.Parse(time.RFC3339, post.Updated)
			if err != nil {
				return err
			}
			updatedSeconds = updatedTime.Unix()
		}

		documents[i] = Document{
			ID:            i,
			Body:          post.Markdown,
			Summary:       post.Summary,
			Title:         post.Title,
			URL:           post.URL,
			PublishedTs:   publishedTime.Unix(),
			UpdatedTs:     updatedSeconds,
			PublishedYear: publishedYear,
			Tags:          post.Tags,
		}
	}

	task, err := s.client.Index(IndexName).AddDocuments(documents, nil)
	if err != nil {
		return err
	}
	if err := s.waitForTask(task.TaskUID); err != nil {
		return err
	}

	publishedYears = unique(publishedYears)
	slices.SortFunc(publishedYears, func(i, j int) int {
		return cmp.Compare(j, i)
	})
	s.setPublishedYears(publishedYears)

	return nil
}

var attributesToRetrieve = []string{
	"title",
	"summary",
	"url",
	"publishedTs",
	"updatedTs",
	"tags",
}

func (s *SearchService) SearchPostsOfYear(year int) ([]PostMetadata, error) {
	request := meilisearch.SearchRequest{
		Filter:               "publishedYear=" + strconv.Itoa(year),
		Limit:                9_000,
		AttributesToRetrieve: attributesToRetrieve,
	}
	response, err := s.client.Index(IndexName).Search("", &request)
	if err != nil {
		return nil, err
	}

	return mapToPostMetadata(response)
}

func (s *SearchService) SearchWithTag(tag string) ([]PostMetadata, error) {
	request := meilisearch.SearchRequest{
		Filter:               tagFilter(tag),
		Limit:                9_000,
		AttributesToRetrieve: attributesToRetrieve,
	}
	response, err := s.client.Index(IndexName).Search("", &request)
	if err != nil {
		return nil, err
	}

	return mapToPostMetadata(response)
}

func tagFilter(tag string) string {
	return `tags = "` + escapeFilterValue(tag) + `"`
}

func escapeFilterValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func (s *SearchService) Search(query string) ([]PostMetadata, error) {
	request := meilisearch.SearchRequest{
		Limit:                9_000,
		AttributesToRetrieve: attributesToRetrieve,
	}
	response, err := s.client.Index(IndexName).Search(query, &request)
	if err != nil {
		return nil, err
	}

	return mapToPostMetadata(response)
}

func mapToPostMetadata(response *meilisearch.SearchResponse) ([]PostMetadata, error) {
	posts := make([]PostMetadata, 0, response.Hits.Len())
	documentHits := make([]Document, 0)
	err := response.Hits.DecodeInto(&documentHits)
	if err != nil {
		return nil, fmt.Errorf("decode search hits: %w", err)
	}
	for _, document := range documentHits {
		published := ""
		updated := ""
		var publishedTS time.Time
		if document.PublishedTs != 0 {
			publishedTS = time.Unix(document.PublishedTs, 0)
			published = publishedTS.Format("2. January 2006")
		}
		if document.UpdatedTs != 0 {
			updatedTime := time.Unix(document.UpdatedTs, 0)
			updated = updatedTime.Format("2. January 2006")
		}

		var tags []string
		if document.Tags != nil {
			tagsList := document.Tags
			tags = append(tags, tagsList...)
		}

		posts = append(posts, PostMetadata{
			Title:       document.Title,
			Summary:     document.Summary,
			URL:         document.URL,
			Published:   published,
			PublishedTS: publishedTS,
			Updated:     updated,
			Tags:        tags,
		})
	}
	return posts, nil
}

func sortPostsNewest(posts []PostMetadata) {
	slices.SortFunc(posts, func(a, b PostMetadata) int {
		return b.PublishedTS.Compare(a.PublishedTS)
	})
}
