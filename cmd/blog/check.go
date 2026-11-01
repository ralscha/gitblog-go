package main

import (
	"errors"
	"fmt"
	"gitblog/assets"
	"html/template"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

func (app *application) checkBrokenLinks() error {
	started := time.Now()
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

	lastReportedProgress := 0
	lastReport := time.Now()
	app.logger.Info("collected links", "urls", len(targets))
	failedDomains := make(map[string]string)
	for i, target := range targets {
		app.logger.Debug("checking link", "url", target.URL, "checked", i, "total", len(targets))
		target = checkLink(&httpClient, target, failedDomains)
		if target.Error != "" || target.Status != 0 {
			urlChecks = append(urlChecks, target)
			app.logger.Warn("broken link", "url", target.URL, "status", target.Status, "error", target.Error)
		}
		progress := ((i + 1) * 100) / len(targets)
		if progress >= lastReportedProgress+10 || time.Since(lastReport) >= 30*time.Second || i+1 == len(targets) {
			app.logger.Info("link-check progress", "percent", progress, "checked", i+1, "total", len(targets), "elapsed", time.Since(started))
			lastReportedProgress = progress
			lastReport = time.Now()
		}
	}

	if len(urlChecks) == 0 {
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

	app.logger.Info("link-check completed", "checked", len(targets), "broken", len(urlChecks), "elapsed", time.Since(started), "report", reportFile)
	return nil
}

func checkLink(client *http.Client, target URLCheck, failedDomains map[string]string) URLCheck {
	req, err := http.NewRequest(http.MethodGet, target.URL, nil)
	if err != nil {
		target.Error = err.Error()
		return target
	}
	domain := req.URL.Hostname()
	if message, ok := failedDomains[domain]; ok {
		target.Error = message
		return target
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("User-Agent", "gitblog-link-checker/1.0")
	resp, err := client.Do(req)
	if err != nil {
		target.Error = err.Error()
		if dnsErr, ok := errors.AsType[*net.DNSError](err); ok && dnsErr.IsNotFound {
			failedDomains[domain] = dnsErr.Error()
		}
		return target
	}
	// Only headers are needed. Reading even a limited body can wait on slow
	// streams; upgraded (101) connections are not bounded by Client.Timeout.
	_ = resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 || resp.StatusCode == http.StatusTooManyRequests {
		return target
	}
	target.Status = resp.StatusCode
	target.Location = resp.Header.Get("Location")
	return target
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
