package main

import (
	"bytes"
	"fmt"
	"golang.org/x/net/html"
	htmltemplate "html/template"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var headerPattern = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n---\r?\n?(.*)\z`)

func (app *application) convert(markdownFile string) error {
	header, body, err := readPostSource(markdownFile)
	if err != nil {
		_, _ = removeGeneratedHTML(siblingPath(markdownFile, "html"))
		return err
	}
	if header.Draft {
		_, err := removeGeneratedHTML(siblingPath(markdownFile, "html"))
		return err
	}
	published, updated, err := validatePostHeader(header, markdownFile)
	if err != nil {
		_, _ = removeGeneratedHTML(siblingPath(markdownFile, "html"))
		return err
	}

	codeBody, err := app.gitHubCodeService.InsertCode(body)
	if err != nil {
		return fmt.Errorf("failed to insert code: %w", err)
	}

	htmlContent, err := app.markdownService.Convert(codeBody)
	if err != nil {
		return fmt.Errorf("failed to convert markdown: %w", err)
	}

	htmlContent, err = addTargetBlankToLinks(htmlContent)
	if err != nil {
		return fmt.Errorf("failed to add target blank to links: %w", err)
	}

	htmlContent, err = app.shiki(htmlContent)
	if err != nil {
		return fmt.Errorf("failed to shiki: %w", err)
	}

	htmlContent = strings.TrimPrefix(htmlContent, "<html><head></head><body>")
	htmlContent = strings.TrimSuffix(htmlContent, "</body></html>")

	url, err := filepath.Rel(app.config.Blog.PostDir, siblingPath(markdownFile, "html"))
	if err != nil {
		return fmt.Errorf("failed to get relative path: %w", err)
	}
	url = filepath.ToSlash(url)
	feedbackURL := strings.ReplaceAll(url, "/", "-")

	var postUpdated string
	if !updated.IsZero() {
		postUpdated = updated.Format("2. January 2006")
	}

	post := Post{
		SiteTitle:   app.config.Blog.Title,
		Title:       header.Title,
		HTML:        htmltemplate.HTML(htmlContent), // Trusted Markdown from the configured Git repository.
		Published:   published.Format("2. January 2006"),
		Updated:     postUpdated,
		Tags:        header.Tags,
		FeedbackURL: feedbackURL,
		URL:         absoluteBlogURL(app.config.Blog.URL, url),
	}

	var output bytes.Buffer
	err = app.templates["post"].Execute(&output, post)
	if err != nil {
		return fmt.Errorf("failed to execute post template: %w", err)
	}

	htmlPath := siblingPath(markdownFile, "html")
	if err := writeFileAtomic(htmlPath, output.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write html file: %w", err)
	}

	if err := compressFileWithGzip(htmlPath); err != nil {
		return fmt.Errorf("failed to gzip html file: %w", err)
	}

	if err := compressFileWithBrotli(htmlPath); err != nil {
		return fmt.Errorf("failed to brotli html file: %w", err)
	}

	return nil
}

func addTargetBlankToLinks(htmlStr string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return "", err
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" && hasExternalHref(n) {
			setAttr(n, "target", "_blank")
			setAttr(n, "rel", "noopener noreferrer")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	var b strings.Builder
	err = html.Render(&b, doc)
	if err != nil {
		return "", err
	}

	return b.String(), nil
}

func (app *application) shiki(htmlContent string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}

	var f func(*html.Node) error
	f = func(n *html.Node) error {
		if n.Type == html.ElementNode && n.Data == "code" {
			for _, a := range n.Attr {
				if strings.HasPrefix(a.Key, "class") && strings.Contains(a.Val, "language-") {
					lang := "markup"
					classes := strings.FieldsSeq(a.Val)
					for cl := range classes {
						if after, ok := strings.CutPrefix(cl, "language-"); ok {
							lang = after
							break
						}
					}

					code, err := app.runShiki(lang, nodeText(n))
					if err != nil {
						return err
					}
					codeNode, err := html.ParseFragment(strings.NewReader(code), n)
					if err != nil {
						return err
					}

					n.FirstChild = nil
					n.LastChild = nil
					for _, c := range codeNode {
						n.AppendChild(c)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			err := f(c)
			if err != nil {
				return err
			}
		}
		return nil
	}

	err = f(doc)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", err
	}

	cleaned, err := cleanupShikiOutput(buf.String())
	if err != nil {
		return "", fmt.Errorf("failed to cleanup shiki output: %w", err)
	}

	return cleaned, nil
}

func cleanupShikiOutput(htmlContent string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}

	extract := func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode {
			if n.Data == "pre" {
				// Check if this pre contains a code that contains a shiki pre
				if code := findFirstChild(n, "code"); code != nil {
					if shikiPre := findFirstChild(code, "pre"); shikiPre != nil {
						if hasShikiClass(shikiPre) {
							return shikiPre
						}
					}
				}
			}
		}
		return nil
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "pre" {
			if replacement := extract(n); replacement != nil {
				// Replace the current node's attributes and children with the shiki pre
				n.Attr = replacement.Attr
				n.FirstChild = replacement.FirstChild
				n.LastChild = replacement.LastChild
			}
		}

		// Continue traversing
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func hasExternalHref(n *html.Node) bool {
	for _, attr := range n.Attr {
		if attr.Key == "href" {
			return strings.HasPrefix(attr.Val, "http://") || strings.HasPrefix(attr.Val, "https://")
		}
	}
	return false
}

func setAttr(n *html.Node, key, value string) {
	for i, attr := range n.Attr {
		if attr.Key == key {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: value})
}

func nodeText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return sb.String()
}

func findFirstChild(n *html.Node, tag string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

func hasShikiClass(n *html.Node) bool {
	for _, attr := range n.Attr {
		if attr.Key == "class" && strings.Contains(attr.Val, "shiki") {
			return true
		}
	}
	return false
}

func (app *application) runShiki(language, code string) (string, error) {
	codeTmp, err := os.CreateTemp("", "code")
	if err != nil {
		return "", fmt.Errorf("failed to create tmp file: %w", err)
	}
	codePath := codeTmp.Name()
	defer func() { _ = os.Remove(codePath) }()
	if _, err := codeTmp.WriteString(code); err != nil {
		_ = codeTmp.Close()
		return "", fmt.Errorf("failed to write code to tmp file: %w", err)
	}
	if err := codeTmp.Close(); err != nil {
		return "", fmt.Errorf("failed to close code tmp file: %w", err)
	}

	outTmp, err := os.CreateTemp("", "out")
	if err != nil {
		return "", fmt.Errorf("failed to create output tmp file: %w", err)
	}
	outPath := outTmp.Name()
	defer func() { _ = os.Remove(outPath) }()
	if err := outTmp.Close(); err != nil {
		return "", fmt.Errorf("failed to close output tmp file: %w", err)
	}

	cmd := exec.Command("node", app.config.Blog.Shikicli, codePath, outPath, language)
	commandOutput, err := cmd.CombinedOutput()
	if err != nil {
		if app.logger != nil {
			app.logger.Warn("Shiki highlighting failed; using plain code", "error", err, "output", string(commandOutput))
		}
		return fmt.Sprintf(`<pre class="shiki"><code>%s</code></pre>`, html.EscapeString(code)), nil
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		return "", fmt.Errorf("failed to read output tmp file: %w", err)
	}

	return string(content), nil
}

func (app *application) convertChangedMarkdowns() (bool, error) {
	markdownFiles, err := app.collectAllMarkdownFiles()
	if err != nil {
		return false, fmt.Errorf("failed to collect markdown files: %w", err)
	}
	changed := false
	for _, markdownFile := range markdownFiles {
		header, _, err := readPostSource(markdownFile)
		if err != nil {
			return false, err
		}
		if header.Draft {
			removed, err := removeGeneratedHTML(siblingPath(markdownFile, "html"))
			if err != nil {
				return false, err
			}
			changed = changed || removed
			continue
		}

		markdownFileInfo, err := os.Stat(markdownFile)
		if err != nil {
			return false, fmt.Errorf("failed to get markdown file info: %w", err)
		}

		htmlFile := siblingPath(markdownFile, "html")
		htmlFileInfo, err := os.Stat(htmlFile)
		if err != nil {
			if !os.IsNotExist(err) {
				return false, fmt.Errorf("failed to get html file info: %w", err)
			}
		} else {
			if htmlFileInfo.ModTime().After(markdownFileInfo.ModTime()) {
				// html file is newer, skip conversion
				continue
			}
		}

		changed = true
		app.logger.Info("converting markdown", "file", markdownFile)
		err = app.convert(markdownFile)
		if err != nil {
			return false, fmt.Errorf("failed to convert markdown: %w", err)
		}
	}

	return changed, nil
}
