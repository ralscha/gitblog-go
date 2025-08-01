package main

import (
	"fmt"
	"gitblog/assets"
	"golang.org/x/net/html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

func (app *application) checkBrokenLinks() {
	fmt.Println("Checking broken links...")

	ignoreUrlsFile, err := os.ReadFile(app.config.Blog.PostDir + "/ignore-urls.txt")
	if err != nil {
		app.logger.Error(err.Error())
		return
	}

	ignoreUrls := strings.Split(string(ignoreUrlsFile), "\n")
	for i, url := range ignoreUrls {
		ignoreUrls[i] = strings.ToLower(url)
	}

	posts, err := app.readAllMetadata()
	if err != nil {
		app.logger.Error(err.Error())
		return
	}

	httpClient := http.Client{
		Timeout: 30 * time.Second,
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
			for _, ignoreURL := range ignoreUrls {
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

	fmt.Printf("Found %d unique URLs to check\n", totalUrls)

	for _, post := range posts {
		htmlFile := siblingPath(post.MarkdownFile, "html")
		htmlContent, err := os.ReadFile(htmlFile)
		if err != nil {
			app.logger.Error(err.Error())
			continue
		}

		links, err := collectLinks(string(htmlContent))
		if err != nil {
			app.logger.Error(err.Error())
			continue
		}

		for _, link := range links {
			ignore := false
			linkLower := strings.ToLower(link)
			for _, ignoreURL := range ignoreUrls {
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
					fmt.Printf("Progress: %d%% (%d/%d URLs checked)\n", progress, checkedCount, totalUrls)
					lastReportedProgress = progress
				}

				req, err := http.NewRequest("GET", cleanedUpLink, nil)
				if err != nil {
					app.logger.Error(err.Error())
					continue
				}

				req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
				req.Header.Set("accept-encoding", "gzip, deflate, br, zstd")
				req.Header.Set("accept-language", "en-US,en;q=0.9,de;q=0.8,th;q=0.7,hu;q=0.6")
				req.Header.Set("cache-control", "max-age=0")
				req.Header.Set("if-modified-since", "Wed, 02 Apr 2025 00:16:58 GMT")
				req.Header.Set("if-none-match", `"d8vpyn1y38q111zf"`)
				req.Header.Set("priority", "u=0, i")
				req.Header.Set("sec-ch-ua", `"Google Chrome";v="135", "Not-A.Brand";v="8", "Chromium";v="135"`)
				req.Header.Set("sec-ch-ua-mobile", "?0")
				req.Header.Set("sec-ch-ua-platform", `"Windows"`)
				req.Header.Set("sec-fetch-dest", "document")
				req.Header.Set("sec-fetch-mode", "navigate")
				req.Header.Set("sec-fetch-site", "none")
				req.Header.Set("sec-fetch-user", "?1")
				req.Header.Set("upgrade-insecure-requests", "1")
				req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36")

				resp, err := httpClient.Do(req)
				if err != nil {
					if strings.Contains(err.Error(), "no such host") {
						failedDomains[domain] = struct{}{}
						app.logger.Error(fmt.Sprintf("Domain %s marked as failed due to 'no such host' error: %s", domain, err.Error()))
					} else {
						app.logger.Error(err.Error())
					}
					continue
				}
				checkedUrls[cleanedUpLink] = struct{}{}

				if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
					continue
				}

				if resp.StatusCode == 429 {
					continue
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
			}
		}
	}

	if len(urlChecks) > 0 {
		fmt.Println("Broken links found:")
		for _, urlCheck := range urlChecks {
			fmt.Printf("  %s: %d\n", urlCheck.URL, urlCheck.Status)
		}
	} else {
		fmt.Println("No broken links found.")
	}

	tmpl, err := template.ParseFS(assets.EmbeddedHTML, "html/urlcheck.tmpl")
	if err != nil {
		app.logger.Error(err.Error())
		return
	}

	var output strings.Builder
	err = tmpl.Execute(&output, urlChecks)
	if err != nil {
		app.logger.Error(err.Error())
		return
	}

	reportFile := app.config.Blog.PostDir + "/report/urlcheck.html"
	err = os.MkdirAll(filepath.Dir(reportFile), os.ModePerm)
	if err != nil {
		app.logger.Error(err.Error())
		return
	}

	err = os.WriteFile(reportFile, []byte(output.String()), 0644)
	if err != nil {
		app.logger.Error(err.Error())
		return
	}

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
	pos := strings.Index(url, "#")
	if pos != -1 {
		return url[:pos]
	}
	return url
}
