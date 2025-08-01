package main

import (
	"bytes"
	"fmt"
	"gitblog/assets"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"
)

var headerPattern = regexp.MustCompile(`(?s)---(.*?)---(.*)`)

func (app *application) convert(markdownFile string) error {
	content, err := os.ReadFile(markdownFile)
	if err != nil {
		return fmt.Errorf("failed to read markdown file: %w", err)
	}

	matcher := headerPattern.FindStringSubmatch(string(content))
	if matcher == nil {
		htmlPath := siblingPath(markdownFile, "html")
		if err := os.Remove(htmlPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete existing html file: %w", err)
		}
		return fmt.Errorf("content does not match header pattern")
	}

	headerString := matcher[1]
	header := PostHeader{}
	if err := yaml.Unmarshal([]byte(headerString), &header); err != nil {
		return fmt.Errorf("failed to unmarshal header: %w", err)
	}

	body := matcher[2]

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

	var postPublished string
	if header.Published != "" {
		published, err := time.Parse(time.RFC3339, header.Published)
		if err != nil {
			return fmt.Errorf("failed to parse published time: %w", err)
		}
		postPublished = published.Format("2. January 2006")
	}

	var postUpdated string
	if header.Updated != "" {
		updated, err := time.Parse(time.RFC3339, header.Updated)
		if err != nil {
			return fmt.Errorf("failed to parse updated time: %w", err)
		}
		postUpdated = updated.Format("2. January 2006")
	}

	post := Post{
		Title:       header.Title,
		HTML:        htmlContent,
		Published:   postPublished,
		Updated:     postUpdated,
		Tags:        header.Tags,
		FeedbackURL: feedbackURL,
		URL:         app.config.Blog.URL + url,
	}

	tmpl := template.Must(template.ParseFS(assets.EmbeddedHTML, "html/post.tmpl"))

	var output bytes.Buffer
	err = tmpl.Execute(&output, post)
	if err != nil {
		return fmt.Errorf("failed to execute post template: %w", err)
	}

	htmlPath := siblingPath(markdownFile, "html")
	if err := os.WriteFile(htmlPath, output.Bytes(), 0644); err != nil {
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
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" && (strings.HasPrefix(a.Val, "http://") || strings.HasPrefix(a.Val, "https://")) {
					n.Attr = append(n.Attr, html.Attribute{Key: "target", Val: "_blank"})
					break
				}
			}
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
					classes := strings.Fields(a.Val)
					for _, cl := range classes {
						if strings.HasPrefix(cl, "language-") {
							lang = strings.TrimPrefix(cl, "language-")
							break
						}
					}

					code, err := app.runShiki(lang, html.UnescapeString(n.FirstChild.Data))
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
	defer func() {
		if err := os.Remove(codeTmp.Name()); err != nil {
			fmt.Printf("failed to remove temp file %s: %v\n", codeTmp.Name(), err)
		}
	}()
	if _, err := codeTmp.WriteString(code); err != nil {
		return "", fmt.Errorf("failed to write code to tmp file: %w", err)
	}

	outTmp, err := os.CreateTemp("", "out")
	if err != nil {
		return "", fmt.Errorf("failed to create output tmp file: %w", err)
	}
	defer func() {
		if err := os.Remove(outTmp.Name()); err != nil {
			fmt.Printf("failed to remove temp file %s: %v\n", outTmp.Name(), err)
		}
	}()

	cmd := exec.Command("node", app.config.Blog.Shikicli, codeTmp.Name(), outTmp.Name(), language)
	_, err = cmd.Output()
	if err != nil {
		fmt.Println("failed to run shiki", err)
		return fmt.Sprintf(`<pre class="shiki"><code>%s</code></pre>`, html.EscapeString(code)), nil
	}

	content, err := os.ReadFile(outTmp.Name())
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
