package main

import "time"

type Post struct {
	Title       string
	HTML        string
	Published   string
	Updated     string
	FeedbackURL string
	URL         string
	Tags        []string
}

type URLCheck struct {
	URL      string
	Post     string
	Status   int
	Location string
}

type YearNavigation struct {
	Year    int
	Current bool
}

type PostMetadata struct {
	Draft        bool
	URL          string
	Markdown     string
	MarkdownFile string
	FeedbackURL  string
	Title        string
	Tags         []string
	Published    string
	PublishedTS  time.Time
	Updated      string
	Summary      string
}

type SearchResults struct {
	Posts []PostMetadata
	Query string
	Years []YearNavigation
}

type PostHeader struct {
	Title     string
	Tags      []string
	Draft     bool
	Summary   string
	Published string
	Updated   string
}
