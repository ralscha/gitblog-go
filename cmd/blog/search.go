package main

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

const (
	IndexName = "posts"
)

type SearchService struct {
	client         meilisearch.ServiceManager
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
		fmt.Printf("decoding published years failed: %v\n", err)
		return nil, err
	}
	for _, year := range years {
		publishedYears = append(publishedYears, year.PublishedYear)
	}

	for _, result := range response.Results {
		var doc Document
		err := result.DecodeInto(&doc)
		if err != nil {
			fmt.Printf("decoding document failed: %v\n", err)
			continue
		}
		publishedYears = append(publishedYears, doc.PublishedYear)
	}
	publishedYears = unique(publishedYears)
	slices.SortFunc(publishedYears, func(i, j int) int {
		return j - i
	})

	return &SearchService{
		client:         c,
		publishedYears: publishedYears,
	}, nil
}

func unique(intSlice []int) []int {
	keys := make(map[int]bool)
	var list []int
	for _, entry := range intSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func (s *SearchService) DeleteAll() error {
	_, err := s.client.Index(IndexName).DeleteAllDocuments()
	if err != nil {
		return err
	}

	return nil
}

func (s *SearchService) IndexPosts(posts []PostMetadata) error {
	documents := make([]Document, len(posts))
	for i, post := range posts {
		publishedTime, err := time.Parse(time.RFC3339, post.Published)
		if err != nil {
			return err
		}
		publishedYear := publishedTime.Year()

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

	_, err := s.client.Index(IndexName).AddDocuments(documents, nil)
	if err != nil {
		return err
	}

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

	posts := s.mapToPostMetadata(response)
	return posts, nil
}

func (s *SearchService) SearchWithTag(tag string) ([]PostMetadata, error) {
	request := meilisearch.SearchRequest{
		Filter:               "tags=" + tag,
		Limit:                9_000,
		AttributesToRetrieve: attributesToRetrieve,
	}
	response, err := s.client.Index(IndexName).Search("", &request)
	if err != nil {
		return nil, err
	}

	posts := s.mapToPostMetadata(response)
	return posts, nil
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

	posts := s.mapToPostMetadata(response)
	return posts, nil
}

func (s *SearchService) mapToPostMetadata(response *meilisearch.SearchResponse) []PostMetadata {
	var posts []PostMetadata
	documentHits := make([]Document, 0)
	err := response.Hits.DecodeInto(&documentHits)
	if err != nil {
		fmt.Printf("decoding hit failed: %v\n", err)
		return posts
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
			tagsList := documentHits[0].Tags
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
	return posts
}
