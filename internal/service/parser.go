package service

import (
	"bytes"
	"strings"

	"url-collector/internal/domain"

	"golang.org/x/net/html"
)

// ParseHTML parses HTML and extracts metadata
func ParseHTML(body []byte) (*domain.Metadata, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	meta := &domain.Metadata{}
	parseNode(doc, meta)
	return meta, nil
}

func parseNode(n *html.Node, meta *domain.Metadata) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "title":
			// Only capture the first title found
			if meta.Title == "" && n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				meta.Title = strings.TrimSpace(n.FirstChild.Data)
			}
		case "meta":
			parseMeta(n, meta)
		}
	}

	// Recurse into children
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		parseNode(c, meta)
	}
}

func parseMeta(n *html.Node, meta *domain.Metadata) {
	var property, name, content string

	for _, attr := range n.Attr {
		switch strings.ToLower(attr.Key) {
		case "property":
			property = attr.Val
		case "name":
			name = attr.Val
		case "content":
			content = attr.Val
		}
	}

	// OGP (property attribute)
	switch property {
	case "og:title":
		meta.OGTitle = content
	case "og:description":
		meta.OGDescription = content
	case "og:image":
		meta.OGImage = content
	case "og:url":
		meta.OGURL = content
	case "og:site_name":
		meta.OGSiteName = content
	}

	// Standard meta (name attribute)
	if name == "description" {
		meta.Description = content
	}
}

// IsHTML checks if content type is HTML
func IsHTML(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "text/html") ||
		strings.Contains(contentType, "application/xhtml+xml")
}
