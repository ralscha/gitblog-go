package main

import (
	"html/template"
	"time"
)

type Post struct {
	SiteTitle   string
	Title       string
	HTML        template.HTML
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
	SiteTitle       string
	SiteDescription string
	Posts           []PostMetadata
	Query           string
	Years           []YearNavigation
}

type FeedbackPage struct {
	SiteTitle string
	PostURL   string
	Token     string
}

type PostHeader struct {
	Title     string
	Tags      []string
	Draft     bool
	Summary   string
	Published string
	Updated   string
}
