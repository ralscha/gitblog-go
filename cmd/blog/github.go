package main

import (
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/patrickmn/go-cache"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	gitHubLink    = "https://github.com/"
	gitHubRawLink = "https://raw.githubusercontent.com/"
	maxGitHubCode = 5 << 20
)

type GitHubCodeService struct {
	httpClient    *http.Client
	siteCache     *cache.Cache
	githubPattern *regexp.Regexp
}

func NewGitHubCodeService() *GitHubCodeService {
	c := cache.New(5*time.Minute, 10*time.Minute)
	pattern := regexp.MustCompile(`\[github:` + regexp.QuoteMeta(gitHubLink) + `((.*?)(?:#L([0-9]+)(?:-L([0-9]+))??)??)(?::(.*?))??\]`)

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 4
	retryClient.Logger = nil
	stdClient := retryClient.StandardClient()
	stdClient.Timeout = 1 * time.Minute

	return &GitHubCodeService{
		httpClient:    stdClient,
		siteCache:     c,
		githubPattern: pattern,
	}
}

func (gcs *GitHubCodeService) InsertCode(markdown string) (string, error) {
	matches := gcs.githubPattern.FindAllStringSubmatchIndex(markdown, -1)
	var sb strings.Builder

	lastIndex := 0
	for _, match := range matches {
		sb.WriteString(markdown[lastIndex:match[0]])

		completeURL := markdown[match[2]:match[3]]
		url := markdown[match[4]:match[5]]

		var from, to int
		var err error
		if match[6] != -1 {
			from, err = strconv.Atoi(markdown[match[6]:match[7]])
			if err != nil || from < 1 {
				return "", fmt.Errorf("invalid starting line in %s", completeURL)
			}
			to = from
		}
		if match[8] != -1 {
			to, err = strconv.Atoi(markdown[match[8]:match[9]])
			if err != nil || to < from {
				return "", fmt.Errorf("invalid ending line in %s", completeURL)
			}
		}

		parts := strings.SplitN(url, "/", 4)
		if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] != "blob" || !strings.Contains(parts[3], "/") {
			return "", fmt.Errorf("invalid GitHub file URL: %s", completeURL)
		}
		url = parts[0] + "/" + parts[1] + "/" + parts[3]

		language := ""
		if match[10] != -1 {
			language = markdown[match[10]:match[11]]
		} else {
			lastDot := strings.LastIndex(url, ".")
			if lastDot != -1 {
				language = url[lastDot+1:]
			}
		}

		if strings.ContainsAny(language, "`\r\n") {
			return "", fmt.Errorf("invalid code language %q", language)
		}
		code, err := gcs.fetchCode(gitHubRawLink + url)
		if err != nil {
			return "", err
		}

		if code != "" {
			var replacementCode string
			if from != 0 && to != 0 {
				lines, err := getLines(code, from, to)
				if err != nil {
					return "", fmt.Errorf("select lines from %s: %w", completeURL, err)
				}
				replacementCode = strings.Join(lines, "\n")
			} else {
				replacementCode = code
			}
			replacementCode = strings.ReplaceAll(replacementCode, "\t", "  ")

			fileName := url
			lastSlash := strings.LastIndex(url, "/")
			if lastSlash != -1 {
				fileName = url[lastSlash+1:]
			}

			fence := codeFence(replacementCode)
			replacement := fmt.Sprintf("\n%s%s\n%s\n%s\n<small class=\"gh\">[%s](%s%s)</small>\n",
				fence, language, replacementCode, fence, fileName, gitHubLink, completeURL)

			sb.WriteString(replacement)
		}

		lastIndex = match[1]
	}
	sb.WriteString(markdown[lastIndex:])

	return sb.String(), nil
}

// A fetched Markdown file can itself contain fences. Use a longer delimiter so
// its contents always remain source code, including any embedded HTML.
func codeFence(code string) string {
	longest, run := 2, 0
	for _, char := range code {
		if char == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return strings.Repeat("`", longest+1)
}

func getLines(code string, from, to int) ([]string, error) {
	lines := strings.Split(code, "\n")
	if from < 1 || from > len(lines) || to < from {
		return nil, fmt.Errorf("invalid line range %d-%d for a %d-line file", from, to, len(lines))
	}
	if to > len(lines) {
		to = len(lines)
	}
	return lines[from-1 : to], nil
}

func (gcs *GitHubCodeService) fetchCode(url string) (string, error) {
	if code, found := gcs.siteCache.Get(url); found {
		return code.(string), nil
	}

	resp, err := gcs.httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch code, status code: %d", resp.StatusCode)
	}

	if resp.ContentLength > maxGitHubCode {
		return "", fmt.Errorf("GitHub code response is too large: %d bytes", resp.ContentLength)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGitHubCode+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxGitHubCode {
		return "", fmt.Errorf("GitHub code response exceeds %d bytes", maxGitHubCode)
	}

	code := string(body)
	gcs.siteCache.Set(url, code, cache.DefaultExpiration)
	return code, nil
}
