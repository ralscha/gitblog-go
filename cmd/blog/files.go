package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/andybalholm/brotli"
)

func siblingPath(filePath, newExt string) string {
	ext := filepath.Ext(filePath)
	base := filePath[:len(filePath)-len(ext)]
	return base + "." + newExt
}

func compressFileWithBrotli(filePath string) error {
	return compressFile(filePath, ".br", func(w io.Writer) (io.WriteCloser, error) {
		return brotli.NewWriter(w), nil
	})
}

func (app *application) collectAllMarkdownFiles() ([]string, error) {
	var markdownFiles []string

	err := filepath.Walk(app.config.Blog.PostDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking directory: %w", err)
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		if info.Name() == "DRAFT.md" {
			return nil
		}

		if !info.IsDir() && isMarkdownFile(path) {
			markdownFiles = append(markdownFiles, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory: %w", err)
	}

	return markdownFiles, nil
}

func isMarkdownFile(path string) bool {
	return filepath.Ext(path) == ".md"
}

func isHTMLFile(path string) bool {
	return filepath.Ext(path) == ".html"
}

func removeGeneratedHTML(htmlPath string) (bool, error) {
	removed := false
	for _, path := range []string{htmlPath, htmlPath + ".gz", htmlPath + ".br"} {
		if err := os.Remove(path); err != nil {
			if !os.IsNotExist(err) {
				return false, fmt.Errorf("remove generated file %s: %w", path, err)
			}
			continue
		}
		removed = true
	}
	return removed, nil
}

func compressFileWithGzip(filePath string) error {
	return compressFile(filePath, ".gz", func(w io.Writer) (io.WriteCloser, error) {
		return gzip.NewWriterLevel(w, gzip.BestCompression)
	})
}

func compressFile(filePath, suffix string, newWriter func(io.Writer) (io.WriteCloser, error)) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read source file: %w", err)
	}

	var compressed bytes.Buffer
	writer, err := newWriter(&compressed)
	if err != nil {
		return fmt.Errorf("create compressor: %w", err)
	}
	if _, err := writer.Write(content); err != nil {
		_ = writer.Close()
		return fmt.Errorf("compress file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish compressed file: %w", err)
	}

	if err := writeFileAtomic(filePath+suffix, compressed.Bytes(), 0644); err != nil {
		return fmt.Errorf("write compressed file: %w", err)
	}
	return nil
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
