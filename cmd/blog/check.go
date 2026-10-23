package main

import (
	"errors"
	"fmt"
	"gitblog/assets"
	"html/template"
	"io"
	"net"
	"net/http"
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

	seen := make(map[string]struct{})
	var targets []URLCheck
	var urlChecks []URLCheck

	// Read each page once so publication cannot change the list between counting
	// and checking. Keep the first referring post for each unique URL.
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

			if strings.HasPrefix(linkLower, "http://") || strings.HasPrefix(linkLower, "https://") {
				cleanedUpLink := removeFragment(link)
				if _, ok := seen[cleanedUpLink]; !ok {
					seen[cleanedUpLink] = struct{}{}
					targets = append(targets, URLCheck{URL: cleanedUpLink, Post: post.URL})
				}
			}
		}
	}

	lastReportedProgress := -1
	app.logger.Info("collected links", "urls", len(targets))
	failedDomains := make(map[string]string)
	for i, target := range targets {
		progress := ((i + 1) * 100) / len(targets)
		if progress >= lastReportedProgress+10 {
			app.logger.Info("link-check progress", "percent", progress, "checked", i+1, "total", len(targets))
			lastReportedProgress = progress
		}
		req, err := http.NewRequest(http.MethodGet, target.URL, nil)
		if err != nil {
			target.Error = err.Error()
			urlChecks = append(urlChecks, target)
			continue
		}
		domain := req.URL.Hostname()
		if message, ok := failedDomains[domain]; ok {
			target.Error = message
			urlChecks = append(urlChecks, target)
			continue
		}
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("User-Agent", "gitblog-link-checker/1.0")
		resp, err := httpClient.Do(req)
		if err != nil {
			target.Error = err.Error()
			if dnsErr, ok := errors.AsType[*net.DNSError](err); ok && dnsErr.IsNotFound {
				failedDomains[domain] = dnsErr.Error()
			}
			urlChecks = append(urlChecks, target)
			continue
		}
		// Drain small responses for connection reuse, without downloading large
		// archives or other linked files just to inspect their status.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode <= 299 || resp.StatusCode == http.StatusTooManyRequests {
			continue
		}
		target.Status = resp.StatusCode
		target.Location = resp.Header.Get("Location")
		urlChecks = append(urlChecks, target)
	}

	if len(urlChecks) > 0 {
		for _, urlCheck := range urlChecks {
			app.logger.Warn("broken link", "url", urlCheck.URL, "status", urlCheck.Status, "error", urlCheck.Error)
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
