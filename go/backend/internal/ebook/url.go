package ebook

import (
	"fmt"
	"net/url"
	"strings"
)

// deriveURLs parses the input URL and returns repo name, GitHub source URL, and web URL
// Supports both Standard Ebooks URLs and GitHub URLs
func deriveURLs(inputURL string) (repoName, sourceURL, webURL string, err error) {
	parsed, err := url.Parse(inputURL)
	if err != nil {
		return "", "", "", fmt.Errorf("parse URL: %w", err)
	}

	switch parsed.Host {
	case "standardebooks.org", "www.standardebooks.org":
		return deriveFromStandardEbooksURL(parsed)
	case "github.com", "www.github.com":
		return deriveFromGitHubURL(parsed)
	default:
		return "", "", "", fmt.Errorf("unsupported host: %s (expected standardebooks.org or github.com)", parsed.Host)
	}
}

func deriveFromStandardEbooksURL(parsed *url.URL) (repoName, sourceURL, webURL string, err error) {
	// URL format: https://standardebooks.org/ebooks/author/title
	// e.g., https://standardebooks.org/ebooks/mary-shelley/frankenstein
	path := strings.TrimPrefix(parsed.Path, "/ebooks/")
	path = strings.TrimSuffix(path, "/")

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid Standard Ebooks URL path: %s", parsed.Path)
	}

	// Construct repo name: author_title (may have additional parts like translator)
	repoName = strings.Join(parts, "_")

	sourceURL = fmt.Sprintf("https://github.com/standardebooks/%s", repoName)
	webURL = fmt.Sprintf("https://standardebooks.org/ebooks/%s", strings.Join(parts, "/"))

	return repoName, sourceURL, webURL, nil
}

func deriveFromGitHubURL(parsed *url.URL) (repoName, sourceURL, webURL string, err error) {
	// URL format: https://github.com/standardebooks/repo-name
	// e.g., https://github.com/standardebooks/mary-shelley_frankenstein
	path := strings.TrimPrefix(parsed.Path, "/standardebooks/")
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimSuffix(path, ".git")

	if path == "" || strings.Contains(path, "/") {
		return "", "", "", fmt.Errorf("invalid GitHub URL path for Standard Ebooks: %s", parsed.Path)
	}

	repoName = path
	sourceURL = fmt.Sprintf("https://github.com/standardebooks/%s", repoName)

	// Convert repo name to web URL path (replace _ with /)
	webPath := strings.ReplaceAll(repoName, "_", "/")
	webURL = fmt.Sprintf("https://standardebooks.org/ebooks/%s", webPath)

	return repoName, sourceURL, webURL, nil
}

// deriveID extracts a short ID from the repo name
// e.g., "mary-shelley_frankenstein" -> "frankenstein"
func deriveID(repoName string) string {
	parts := strings.Split(repoName, "_")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return repoName
}
