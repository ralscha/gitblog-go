package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"go.abhg.dev/goldmark/anchor"
	"go.abhg.dev/goldmark/mermaid"
)

type dockerCLI struct{}

func (c *dockerCLI) CommandContext(ctx context.Context, args ...string) *exec.Cmd {
	// docker run --rm -u `id -u`:`id -g` -v /path/to/diagrams:/data minlag/mermaid-cli -i diagram.mmd
	if len(args) < 2 {
		path := "docker"
		args = append([]string{"run", "--rm", "minlag/mermaid-cli"}, args...)
		return exec.CommandContext(ctx, path, args...)
	}

	inputPath := args[1]
	baseDir := filepath.Dir(inputPath)
	if baseDir == "." {
		baseDir = ""
	}

	if len(args) > 3 {
		outputDir := filepath.Dir(args[3])
		if outputDir != "." && outputDir != "" && outputDir != baseDir {
			baseDir = filepath.Dir(outputDir)
		}
	}

	if runtime.GOOS != "windows" {
		_ = exec.Command("chmod", "666", inputPath).Run()
	}

	args[1] = filepath.Base(inputPath)
	if len(args) > 3 {
		if runtime.GOOS != "windows" {
			_ = exec.Command("chmod", "666", args[3]).Run()
		}
		args[3] = filepath.Base(args[3])
	}

	path := "docker"
	if baseDir != "" {
		args = append([]string{"run", "--rm", "-v", baseDir + ":/data", "minlag/mermaid-cli"}, args...)
	} else {
		args = append([]string{"run", "--rm", "minlag/mermaid-cli"}, args...)
	}
	return exec.CommandContext(ctx, path, args...)
}

type MarkdownService struct {
	md goldmark.Markdown
}

func NewMarkdownService() *MarkdownService {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM,
			extension.DefinitionList,
			extension.Footnote,
			extension.TaskList,
			&mermaid.Extender{
				RenderMode: mermaid.RenderModeServer,
				CLI:        &dockerCLI{},
			},
			&anchor.Extender{},
			emoji.Emoji),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)

	return &MarkdownService{
		md: md,
	}
}

func (ms *MarkdownService) Convert(markdown string) (string, error) {
	var sb strings.Builder
	if err := ms.md.Convert([]byte(markdown), &sb); err != nil {
		return "", err
	}
	return sb.String(), nil
}
