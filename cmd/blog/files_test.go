package main

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestWriteFileAtomicReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.txt")
	writeTestFile(t, path, "old")
	if err := writeFileAtomic(path, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new" {
		t.Fatalf("content = %q", content)
	}
}

func TestCompressionRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "post.html")
	want := []byte("repeated content repeated content repeated content")
	if err := os.WriteFile(path, want, 0644); err != nil {
		t.Fatal(err)
	}
	if err := compressFileWithGzip(path); err != nil {
		t.Fatal(err)
	}
	if err := compressFileWithBrotli(path); err != nil {
		t.Fatal(err)
	}

	gzipFile, err := os.Open(path + ".gz")
	if err != nil {
		t.Fatal(err)
	}
	gzipReader, err := gzip.NewReader(gzipFile)
	if err != nil {
		t.Fatal(err)
	}
	assertReaderContent(t, gzipReader, want)
	if err := gzipReader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipFile.Close(); err != nil {
		t.Fatal(err)
	}

	brotliFile, err := os.Open(path + ".br")
	if err != nil {
		t.Fatal(err)
	}
	assertReaderContent(t, brotli.NewReader(brotliFile), want)
	if err := brotliFile.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertReaderContent(t *testing.T, reader io.Reader, want []byte) {
	t.Helper()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("content = %q, want %q", got, want)
	}
}
