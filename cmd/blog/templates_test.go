package main

import (
	"bytes"
	"gitblog/assets"
	"html/template"
	"strings"
	"testing"
)

func TestIndexTemplateEscapesUntrustedValues(t *testing.T) {
	tmpl := template.Must(template.ParseFS(assets.EmbeddedHTML, "html/index.tmpl"))
	data := SearchResults{
		SiteTitle:       `<img src=x onerror=alert(1)>`,
		SiteDescription: `<script>alert(1)</script>`,
		Query:           `"><script>alert(1)</script>`,
		Posts: []PostMetadata{{
			Title: `<script>alert(2)</script>`,
			URL:   `post.html`,
			Tags:  []string{`"><img src=x onerror=alert(3)>`},
		}},
	}

	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, unsafe := range []string{"<script>alert(1)</script>", "<script>alert(2)</script>", "<img src=x onerror=alert(3)>"} {
		if strings.Contains(html, unsafe) {
			t.Fatalf("template emitted unescaped input %q", unsafe)
		}
	}
}

func TestPostTemplateKeepsRenderedMarkdownButEscapesMetadata(t *testing.T) {
	tmpl := template.Must(template.ParseFS(assets.EmbeddedHTML, "html/post.tmpl"))
	data := Post{
		SiteTitle: `<b>unsafe site</b>`,
		Title:     `<script>alert(1)</script>`,
		HTML:      template.HTML(`<p><strong>rendered Markdown</strong></p>`),
		URL:       "https://example.com/post.html",
	}

	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if strings.Contains(html, data.Title) || strings.Contains(html, data.SiteTitle) {
		t.Fatal("post metadata was not escaped")
	}
	if !strings.Contains(html, string(data.HTML)) {
		t.Fatal("trusted rendered Markdown was escaped")
	}
}
