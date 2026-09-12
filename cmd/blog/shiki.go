package main

import (
	"fmt"
	"strings"

	shiki "github.com/ralscha/shiki-go"
	"golang.org/x/net/html"
)

func newHighlighter() (*shiki.Highlighter, error) {
	return shiki.NewHighlighter(shiki.HighlighterOptions{
		Themes: []string{"one-light", "one-dark-pro"},
	})
}

func (app *application) shiki(htmlContent string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}

	var walk func(*html.Node) error
	walk = func(n *html.Node) error {
		if n.Type == html.ElementNode && n.Data == "pre" {
			if code := findFirstChild(n, "code"); code != nil {
				if language := codeLanguage(code); language != "" {
					highlighted := app.runShiki(language, nodeText(code))
					nodes, err := html.ParseFragment(strings.NewReader(highlighted), n.Parent)
					if err != nil {
						return err
					}
					if len(nodes) != 1 || nodes[0].Type != html.ElementNode || nodes[0].Data != "pre" {
						return fmt.Errorf("unexpected syntax highlighter output for %q", language)
					}

					// Replace the whole code block, keeping the surrounding document intact.
					n.Attr = nodes[0].Attr
					for n.FirstChild != nil {
						n.RemoveChild(n.FirstChild)
					}
					for nodes[0].FirstChild != nil {
						child := nodes[0].FirstChild
						nodes[0].RemoveChild(child)
						n.AppendChild(child)
					}
					return nil
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(doc); err != nil {
		return "", err
	}

	var output strings.Builder
	if err := html.Render(&output, doc); err != nil {
		return "", err
	}
	return output.String(), nil
}

func codeLanguage(code *html.Node) string {
	for _, attr := range code.Attr {
		if attr.Key == "class" {
			for class := range strings.FieldsSeq(attr.Val) {
				if language, ok := strings.CutPrefix(class, "language-"); ok {
					return language
				}
			}
		}
	}
	return ""
}

func (app *application) runShiki(language, code string) string {
	// Loading an already registered language is a no-op; grammars are reused
	// across code blocks and posts for the lifetime of the application.
	err := app.highlighter.LoadLanguage(language)
	var highlighted string
	if err == nil {
		highlighted, err = app.highlighter.CodeToHTML(code, shiki.Options{
			Lang: language,
			Themes: map[string]any{
				"light": "one-light",
				"dark":  "one-dark-pro",
			},
			DefaultColor: "light",
		})
	}
	if err != nil {
		if app.logger != nil {
			app.logger.Warn("Shiki highlighting failed; using plain code", "language", language, "error", err)
		}
		return fmt.Sprintf(`<pre class="shiki"><code>%s</code></pre>`, html.EscapeString(code))
	}
	return highlighted
}
