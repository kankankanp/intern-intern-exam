package service

import (
	"testing"
)

func TestParseHTML(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		wantTitle         string
		wantDescription   string
		wantOGTitle       string
		wantOGDescription string
		wantOGImage       string
		wantOGURL         string
		wantOGSiteName    string
	}{
		{
			name: "basic title",
			html: `<!DOCTYPE html>
<html>
<head><title>Test Page Title</title></head>
<body></body>
</html>`,
			wantTitle: "Test Page Title",
		},
		{
			name: "title with whitespace",
			html: `<html><head><title>   Title with spaces   </title></head></html>`,
			wantTitle: "Title with spaces",
		},
		{
			name: "meta description",
			html: `<html><head>
<meta name="description" content="This is the description">
</head></html>`,
			wantDescription: "This is the description",
		},
		{
			name: "full OGP",
			html: `<html><head>
<title>Page Title</title>
<meta name="description" content="Meta description">
<meta property="og:title" content="OG Title">
<meta property="og:description" content="OG Description">
<meta property="og:image" content="https://example.com/image.png">
<meta property="og:url" content="https://example.com/page">
<meta property="og:site_name" content="Example Site">
</head></html>`,
			wantTitle:         "Page Title",
			wantDescription:   "Meta description",
			wantOGTitle:       "OG Title",
			wantOGDescription: "OG Description",
			wantOGImage:       "https://example.com/image.png",
			wantOGURL:         "https://example.com/page",
			wantOGSiteName:    "Example Site",
		},
		{
			name:     "empty html",
			html:     `<html><head></head><body></body></html>`,
			wantTitle: "",
		},
		{
			name:     "malformed html",
			html:     `<html><head><title>Unclosed`,
			wantTitle: "Unclosed",
		},
		{
			name: "og with name attribute should not match",
			html: `<html><head>
<meta name="og:title" content="Wrong OG">
<meta property="og:title" content="Correct OG">
</head></html>`,
			wantOGTitle: "Correct OG",
		},
		{
			name: "case insensitive property",
			html: `<html><head>
<meta PROPERTY="og:title" CONTENT="OG Title">
</head></html>`,
			wantOGTitle: "OG Title",
		},
		{
			name: "nested elements",
			html: `<html>
<head>
<title>Outer Title</title>
</head>
<body>
<div>
<title>Inner Title (ignored)</title>
</div>
</body>
</html>`,
			wantTitle: "Outer Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseHTML([]byte(tt.html))
			if err != nil {
				t.Fatalf("ParseHTML() error = %v", err)
			}

			if result.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", result.Title, tt.wantTitle)
			}
			if result.Description != tt.wantDescription {
				t.Errorf("Description = %q, want %q", result.Description, tt.wantDescription)
			}
			if result.OGTitle != tt.wantOGTitle {
				t.Errorf("OGTitle = %q, want %q", result.OGTitle, tt.wantOGTitle)
			}
			if result.OGDescription != tt.wantOGDescription {
				t.Errorf("OGDescription = %q, want %q", result.OGDescription, tt.wantOGDescription)
			}
			if result.OGImage != tt.wantOGImage {
				t.Errorf("OGImage = %q, want %q", result.OGImage, tt.wantOGImage)
			}
			if result.OGURL != tt.wantOGURL {
				t.Errorf("OGURL = %q, want %q", result.OGURL, tt.wantOGURL)
			}
			if result.OGSiteName != tt.wantOGSiteName {
				t.Errorf("OGSiteName = %q, want %q", result.OGSiteName, tt.wantOGSiteName)
			}
		})
	}
}

func TestIsHTML(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"text/html", true},
		{"text/html; charset=utf-8", true},
		{"TEXT/HTML", true},
		{"application/xhtml+xml", true},
		{"application/json", false},
		{"text/plain", false},
		{"image/png", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if got := IsHTML(tt.contentType); got != tt.want {
				t.Errorf("IsHTML(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}
