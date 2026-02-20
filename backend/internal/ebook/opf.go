package ebook

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var tocWhitespacePattern = regexp.MustCompile(`\s+`)

// opfPackage represents the OPF package structure
type opfPackage struct {
	XMLName  xml.Name    `xml:"package"`
	Metadata opfMetadata `xml:"metadata"`
	Spine    opfSpine    `xml:"spine"`
}

type opfMetadata struct {
	Title   string     `xml:"title"`
	Creator opfCreator `xml:"creator"`
}

type opfCreator struct {
	Value string `xml:",chardata"`
}

type opfSpine struct {
	ItemRefs []opfItemRef `xml:"itemref"`
}

type opfItemRef struct {
	IDRef string `xml:"idref,attr"`
}

// parseOPF parses the content.opf file and extracts title, author, and spine order
func parseOPF(repoPath string) (title, author string, err error) {
	opfPath := filepath.Join(repoPath, "src", "epub", "content.opf")

	data, err := os.ReadFile(opfPath)
	if err != nil {
		return "", "", fmt.Errorf("read content.opf: %w", err)
	}

	var pkg opfPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return "", "", fmt.Errorf("unmarshal content.opf: %w", err)
	}

	return pkg.Metadata.Title, pkg.Metadata.Creator.Value, nil
}

// parseSpineOrder parses content.opf and returns the ordered list of content filenames
func parseSpineOrder(repoPath string) ([]string, error) {
	opfPath := filepath.Join(repoPath, "src", "epub", "content.opf")

	data, err := os.ReadFile(opfPath)
	if err != nil {
		return nil, fmt.Errorf("read content.opf: %w", err)
	}

	var pkg opfPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("unmarshal content.opf: %w", err)
	}

	var order []string
	for _, item := range pkg.Spine.ItemRefs {
		// IDRef is like "chapter-1.xhtml", extract filename
		filename := item.IDRef
		if !strings.HasSuffix(filename, ".xhtml") {
			filename = filename + ".xhtml"
		}
		order = append(order, filename)
	}

	return order, nil
}

// parseToc parses toc.xhtml and returns a map of filename to title
func parseToc(repoPath string) (map[string]string, error) {
	tocPath := filepath.Join(repoPath, "src", "epub", "toc.xhtml")

	file, err := os.Open(tocPath)
	if err != nil {
		return nil, fmt.Errorf("open toc.xhtml: %w", err)
	}
	defer func() { _ = file.Close() }()

	doc, err := html.Parse(file)
	if err != nil {
		return nil, fmt.Errorf("parse toc.xhtml: %w", err)
	}

	titles := make(map[string]string)
	extractTocEntries(doc, titles)

	return titles, nil
}

func extractTocEntries(n *html.Node, titles map[string]string) {
	// First find the main toc nav (id="toc"), skip landmarks nav
	tocNav := findTocNav(n)
	if tocNav != nil {
		extractLinksFromNav(tocNav, titles)
	}
}

func findTocNav(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "nav" {
		for _, attr := range n.Attr {
			if attr.Key == "id" && attr.Val == "toc" {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if nav := findTocNav(c); nav != nil {
			return nav
		}
	}
	return nil
}

func extractLinksFromNav(n *html.Node, titles map[string]string) {
	if n.Type == html.ElementNode && n.Data == "a" {
		// Get href attribute
		var href string
		for _, attr := range n.Attr {
			if attr.Key == "href" {
				href = attr.Val
				break
			}
		}

		if href != "" {
			// Extract filename from href (e.g., "text/chapter-1.xhtml" -> "chapter-1.xhtml")
			// Also handle anchors like "text/chapter-24.xhtml#walton-in-continuation"
			href = strings.TrimPrefix(href, "text/")
			if idx := strings.Index(href, "#"); idx != -1 {
				href = href[:idx]
			}

			// Get title text
			title := cleanTocText(getNodeText(n))
			if title != "" && href != "" {
				titles[href] = title
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractLinksFromNav(c, titles)
	}
}

func getNodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var result strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		result.WriteString(getNodeText(c))
	}
	return result.String()
}

func cleanTocText(s string) string {
	// Normalize whitespace
	s = tocWhitespacePattern.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
