package main

import (
	"strings"
	"testing"
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
