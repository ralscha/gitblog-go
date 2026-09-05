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
			if err != nil {
				return "", err
			}
		}
		if match[8] != -1 {
			to, err = strconv.Atoi(markdown[match[8]:match[9]])
			if err != nil {
				return "", err
			}
		}

		if from != 0 && to == 0 {
			to = from
		}

		language := ""
		if match[10] != -1 {
			language = markdown[match[10]:match[11]]
		} else {
			lastDot := strings.LastIndex(url, ".")
			if lastDot != -1 {
				language = url[lastDot+1:]
			}
		}

		url = strings.Replace(url, "/blob", "", 1)
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

			replacement := fmt.Sprintf("\n```%s\n%s\n```\n<small class=\"gh\">[%s](%s%s)</small>\n",
				language, replacementCode, fileName, gitHubLink, completeURL)

			sb.WriteString(replacement)
		}

		lastIndex = match[1]
	}
	sb.WriteString(markdown[lastIndex:])

	return sb.String(), nil
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
