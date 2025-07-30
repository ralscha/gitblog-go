package main

import (
	"compress/gzip"
	"fmt"
	"github.com/andybalholm/brotli"
	"io"
	"os"
	"path/filepath"
)

func siblingPath(filePath, newExt string) string {
	ext := filepath.Ext(filePath)
	base := filePath[:len(filePath)-len(ext)]
	return base + "." + newExt
}

func compressFileWithBrotli(filePath string) error {

	sourceFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening source file: %w", err)
	}
	defer func() {
		if err := sourceFile.Close(); err != nil {
			fmt.Printf("failed to close source file: %v\n", err)
		}
	}()

	destFilePath := filePath + ".br"
	destFile, err := os.Create(destFilePath)
	if err != nil {
		return fmt.Errorf("error creating destination file: %w", err)
	}
	defer func() {
		if err := destFile.Close(); err != nil {
			fmt.Printf("failed to close destination file: %v\n", err)
		}
	}()

	brotliWriter := brotli.NewWriter(destFile)
	defer func() {
		if err := brotliWriter.Close(); err != nil {
			fmt.Printf("failed to close brotli writer: %v\n", err)
		}
	}()

	if _, err := io.Copy(brotliWriter, sourceFile); err != nil {
		return fmt.Errorf("error compressing file with Brotli: %w", err)
	}

	return nil
}

func (app *application) collectAllMarkdownFiles() ([]string, error) {
	var markdownFiles []string

	err := filepath.Walk(app.config.Blog.PostDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking directory: %w", err)
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

func compressFileWithGzip(filePath string) error {
	srcFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			fmt.Printf("failed to close source file: %v\n", err)
		}
	}()

	destPath := filePath + ".gz"
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer func() {
		if err := destFile.Close(); err != nil {
			fmt.Printf("failed to close destination file: %v\n", err)
		}
	}()

	gzWriter, err := gzip.NewWriterLevel(destFile, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("failed to create gzip writer: %w", err)
	}
	defer func() {
		if err := gzWriter.Close(); err != nil {
			fmt.Printf("failed to close gzip writer: %v\n", err)
		}
	}()

	if _, err := io.Copy(gzWriter, srcFile); err != nil {
		return fmt.Errorf("failed to compress and write file: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return nil
}
