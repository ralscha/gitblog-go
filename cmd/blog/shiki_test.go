package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestShikiHighlightsMarkdownCodeBlocks(t *testing.T) {
	highlighter, err := newHighlighter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = highlighter.Close() })
	app := &application{highlighter: highlighter}
	markdown := NewMarkdownService()

	for _, tt := range []struct {
		name     string
		language string
		code     string
		themed   bool
	}{
		{"go", "go", "package main\n\nfunc main() {\n\tprintln(42)\n}\n", true},
		{"alias", "js", "const answer = 42\n", true},
		{"escaped HTML and Unicode", "html", "<script>alert('Grüezi 👋 & < >')</script>\n", true},
		{"plain text alias", "txt", "<tag> & text\n", true},
		{"unknown language", "unknown-language", "<script>alert('x')</script> & text\n", false},
		{"unlabelled", "", "<tag> & text\n", false},
		{"empty", "go", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input, err := markdown.Convert("```" + tt.language + "\n" + tt.code + "```\n")
			if err != nil {
				t.Fatal(err)
			}
			got, err := app.shiki(input)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(got, "<pre") != 1 || strings.Count(got, "<code") != 1 {
				t.Fatalf("expected a single code block: %s", got)
			}
			doc, err := html.Parse(strings.NewReader(got))
			if err != nil {
				t.Fatal(err)
			}
			body := findFirstChild(findFirstChild(doc, "html"), "body")
			code := findFirstChild(findFirstChild(body, "pre"), "code")
			if code == nil || nodeText(code) != tt.code {
				t.Fatalf("code text changed: %s", got)
			}
			if strings.Contains(got, "<script>") || strings.Contains(got, "<tag>") {
				t.Fatalf("code was not escaped: %s", got)
			}
			for _, marker := range []string{"one-light", "one-dark-pro", "--shiki-dark:", "--shiki-dark-bg:"} {
				if strings.Contains(got, marker) != tt.themed {
					t.Fatalf("theme marker %q present = %v, want %v: %s", marker, !tt.themed, tt.themed, got)
				}
			}
			if tt.themed && !strings.Contains(got, "background-color:#FAFAFA") {
				t.Fatalf("light theme is not the default: %s", got)
			}
		})
	}
}

func TestShikiPreservesSurroundingHTML(t *testing.T) {
	highlighter, err := newHighlighter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = highlighter.Close() })
	app := &application{highlighter: highlighter}
	input := `<p>Before <code class="language-go">inline</code></p>` +
		`<blockquote><pre><code class="extra language-go other">package main</code></pre></blockquote>` +
		`<ul><li><pre><code class="language-js">const answer = 42</code></pre></li></ul>` +
		`<pre><code>plain &amp; code</code></pre>` +
		`<pre><code class="other-language-go">also plain</code></pre>` +
		`<pre><code class="language-">empty language</code></pre>` +
		`<svg class="mermaid"><text>Diagram</text></svg><p>After</p>`
	got, err := app.shiki(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		`<p>Before <code class="language-go">inline</code></p>`,
		`<blockquote><pre class="shiki`,
		`</pre></blockquote><ul><li><pre class="shiki`,
		`</pre></li></ul>`,
		`<pre><code>plain &amp; code</code></pre>`,
		`<pre><code class="other-language-go">also plain</code></pre>`,
		`<pre><code class="language-">empty language</code></pre>`,
		`<svg class="mermaid"><text>Diagram</text></svg><p>After</p>`,
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("missing document fragment %q: %s", fragment, got)
		}
	}
	if strings.Count(got, "<pre") != 5 || strings.Count(got, "<code") != 6 {
		t.Fatalf("code blocks were duplicated or nested: %s", got)
	}
}

func TestShikiFallsBackOnHighlighterError(t *testing.T) {
	highlighter, err := newHighlighter()
	if err != nil {
		t.Fatal(err)
	}
	if err := highlighter.Close(); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	app := &application{
		highlighter: highlighter,
		logger:      slog.New(slog.NewTextHandler(&logs, nil)),
	}
	got := app.runShiki("go", "<script> & code\n")
	if want := "<pre class=\"shiki\"><code>&lt;script&gt; &amp; code\n</code></pre>"; got != want {
		t.Fatalf("fallback = %q, want %q", got, want)
	}
	if !strings.Contains(logs.String(), "using plain code") || !strings.Contains(logs.String(), "language=go") {
		t.Fatalf("missing fallback warning: %s", logs.String())
	}
}

func TestConvertHighlightsWithoutNode(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	highlighter, err := newHighlighter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = highlighter.Close() })
	templates, err := loadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	app := &application{
		highlighter:       highlighter,
		markdownService:   NewMarkdownService(),
		gitHubCodeService: NewGitHubCodeService(),
		templates:         templates,
	}
	app.config.Blog.PostDir = t.TempDir()
	app.config.Blog.URL = "https://example.com"
	path := filepath.Join(app.config.Blog.PostDir, "post.md")
	writeTestFile(t, path, "---\ntitle: Highlighted post\npublished: 2026-09-05T12:00:00Z\n---\n"+
		"```go\npackage main\n```\n\n```unknown-language\n<script> & code\n```\n")
	if err := app.convert(path); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(siblingPath(path, "html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<title>Highlighted post</title>", "one-light", "one-dark-pro", "--shiki-dark:", "&lt;script&gt; &amp; code"} {
		if !bytes.Contains(content, []byte(want)) {
			t.Fatalf("converted post is missing %q", want)
		}
	}
	for _, extension := range []string{"html.gz", "html.br"} {
		if _, err := os.Stat(siblingPath(path, extension)); err != nil {
			t.Fatal(err)
		}
	}
}
