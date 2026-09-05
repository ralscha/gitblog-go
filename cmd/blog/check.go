package main

import (
	"errors"
	"fmt"
	"gitblog/assets"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

func (app *application) checkBrokenLinks() error {
	app.logger.Info("checking broken links")

	ignoreURLsFile, err := os.ReadFile(filepath.Join(app.config.Blog.PostDir, "ignore-urls.txt"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read ignored URLs: %w", err)
	}
	ignoreURLs := parseIgnoredURLs(string(ignoreURLsFile))

	posts, err := app.readAllMetadata()
	if err != nil {
		return err
	}

	httpClient := http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	checkedUrls := make(map[string]struct{})
	failedDomains := make(map[string]struct{})
	var urlChecks []URLCheck

	// Collect all unique URLs from posts
	allUniqueUrls := make(map[string]struct{})
	for _, post := range posts {
		htmlFile := siblingPath(post.MarkdownFile, "html")
		htmlContent, err := os.ReadFile(htmlFile)
		if err != nil {
			continue
		}

		links, err := collectLinks(string(htmlContent))
		if err != nil {
			continue
		}

		for _, link := range links {
			ignore := false
			linkLower := strings.ToLower(link)
			for _, ignoreURL := range ignoreURLs {
				if strings.HasPrefix(linkLower, ignoreURL) {
					ignore = true
					break
				}
			}
			if ignore {
				continue
			}

			if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
				cleanedUpLink := removeFragment(link)
				allUniqueUrls[cleanedUpLink] = struct{}{}
			}
		}
	}

	totalUrls := len(allUniqueUrls)
	checkedCount := 0
	lastReportedProgress := -1

	app.logger.Info("collected links", "urls", totalUrls)

	for _, post := range posts {
		htmlFile := siblingPath(post.MarkdownFile, "html")
		htmlContent, err := os.ReadFile(htmlFile)
		if err != nil {
			return fmt.Errorf("read generated post %s: %w", htmlFile, err)
		}

		links, err := collectLinks(string(htmlContent))
		if err != nil {
			return fmt.Errorf("collect links from %s: %w", htmlFile, err)
		}

		for _, link := range links {
			ignore := false
			linkLower := strings.ToLower(link)
			for _, ignoreURL := range ignoreURLs {
				if strings.HasPrefix(linkLower, ignoreURL) {
					ignore = true
					break
				}
			}
			if ignore {
				continue
			}

			if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
				cleanedUpLink := removeFragment(link)
				if _, ok := checkedUrls[cleanedUpLink]; ok {
					continue
				}

				parsedURL, err := url.Parse(cleanedUpLink)
				if err != nil {
					app.logger.Error(err.Error())
					continue
				}
				domain := parsedURL.Host

				// Skip if this domain has already failed with "no such host"
				if _, ok := failedDomains[domain]; ok {
					continue
				}

				checkedCount++
				progress := (checkedCount * 100) / totalUrls
				if progress >= lastReportedProgress+10 {
					app.logger.Info("link-check progress", "percent", progress, "checked", checkedCount, "total", totalUrls)
					lastReportedProgress = progress
				}

				req, err := http.NewRequest("GET", cleanedUpLink, nil)
				if err != nil {
					app.logger.Error(err.Error())
					continue
				}

				req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
				req.Header.Set("User-Agent", "gitblog-link-checker/1.0")

				resp, err := httpClient.Do(req)
				if err != nil {
					if _, ok := errors.AsType[*net.DNSError](err); ok {
						failedDomains[domain] = struct{}{}
						app.logger.Error("domain lookup failed", "domain", domain, "error", err)
					} else {
						app.logger.Error(err.Error())
					}
					continue
				}
				func() {
					defer func() {
						_, _ = io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()
					}()

					checkedUrls[cleanedUpLink] = struct{}{}

					if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
						return
					}

					if resp.StatusCode == 429 {
						return
					}

					if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
						location := resp.Header.Get("Location")
						urlCheck := URLCheck{
							URL:      link,
							Post:     post.URL,
							Status:   resp.StatusCode,
							Location: location,
						}
						urlChecks = append(urlChecks, urlCheck)
					} else {
						urlCheck := URLCheck{
							URL:    link,
							Post:   post.URL,
							Status: resp.StatusCode,
						}
						urlChecks = append(urlChecks, urlCheck)
					}
				}()
			}
		}
	}

	if len(urlChecks) > 0 {
		for _, urlCheck := range urlChecks {
			app.logger.Warn("broken link", "url", urlCheck.URL, "status", urlCheck.Status)
		}
	} else {
		app.logger.Info("no broken links found")
	}

	tmpl, err := template.ParseFS(assets.EmbeddedHTML, "html/urlcheck.tmpl")
	if err != nil {
		return fmt.Errorf("parse URL report template: %w", err)
	}

	var output strings.Builder
	err = tmpl.Execute(&output, urlChecks)
	if err != nil {
		return fmt.Errorf("render URL report: %w", err)
	}

	reportFile := app.config.Blog.PostDir + "/report/urlcheck.html"
	err = os.MkdirAll(filepath.Dir(reportFile), 0755)
	if err != nil {
		return fmt.Errorf("create URL report directory: %w", err)
	}

	err = writeFileAtomic(reportFile, []byte(output.String()), 0644)
	if err != nil {
		return fmt.Errorf("write URL report: %w", err)
	}

	return nil
}

func parseIgnoredURLs(content string) []string {
	var ignored []string
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ignored = append(ignored, strings.ToLower(line))
	}
	return ignored
}

func collectLinks(htmlContent string) ([]string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}
	var links []string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					links = append(links, a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return links, nil
}

func removeFragment(url string) string {
	before, _, ok := strings.Cut(url, "#")
	if ok {
		return before
	}
	return url
}
