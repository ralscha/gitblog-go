package main

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
)

func TestAddTargetBlankToExternalLinks(t *testing.T) {
	got, err := addTargetBlankToLinks(`<p><a href="https://example.com">external</a><a href="/local">local</a></p>`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `href="https://example.com" target="_blank" rel="noopener noreferrer"`) {
		t.Fatalf("external link was not hardened: %s", got)
	}
	if strings.Contains(got, `href="/local" target=`) {
		t.Fatalf("local link was modified: %s", got)
	}
}

func TestConvertChangedMarkdownsRepairsCompressedVariants(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(fmtBool(stale), func(t *testing.T) {
			dir := t.TempDir()
			markdown := filepath.Join(dir, "post.md")
			html := filepath.Join(dir, "post.html")
			writeTestFile(t, markdown, "---\ntitle: Test\npublished: 2026-09-01T00:00:00Z\n---\nBody")
			writeTestFile(t, html, "current HTML")
			old := time.Now().Add(-time.Hour)
			if err := os.Chtimes(markdown, old, old); err != nil {
				t.Fatal(err)
			}
			if stale {
				for _, suffix := range []string{".gz", ".br"} {
					writeTestFile(t, html+suffix, "stale compressed content")
					if err := os.Chtimes(html+suffix, old, old); err != nil {
						t.Fatal(err)
					}
				}
			}
			app := &application{}
			app.config.Blog.PostDir = dir
			if changed, err := app.convertChangedMarkdowns(); err != nil || !changed {
				t.Fatalf("repair = %v, %v", changed, err)
			}
			gz, err := os.Open(html + ".gz")
			if err != nil {
				t.Fatal(err)
			}
			defer gz.Close()
			reader, err := gzip.NewReader(gz)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			assertReaderContent(t, reader, []byte("current HTML"))
			br, err := os.Open(html + ".br")
			if err != nil {
				t.Fatal(err)
			}
			defer br.Close()
			assertReaderContent(t, brotli.NewReader(br), []byte("current HTML"))
			if changed, err := app.convertChangedMarkdowns(); err != nil || changed {
				t.Fatalf("second repair = %v, %v", changed, err)
			}
		})
	}
}

func fmtBool(stale bool) string {
	if stale {
		return "stale"
	}
	return "missing"
}

func TestGetLines(t *testing.T) {
	code := "one\ntwo\nthree"
	lines, err := getLines(code, 2, 9)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(lines, "|"); got != "two|three" {
		t.Fatalf("getLines() = %q", got)
	}
	if _, err := getLines(code, 0, 2); err == nil {
		t.Fatal("getLines() accepted an invalid range")
	}
}
