package feed

import (
	"strings"

	"golang.org/x/net/html"
)

// stripTags removes HTML tags from a string, returning only text content.
func stripTags(body string) string {
	if body == "" {
		return ""
	}
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(body))
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return b.String()
		case html.TextToken:
			b.Write(z.Text())
		}
	}
}

// collapseSpace folds runs of whitespace into single spaces and trims.
func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
		default:
			b.WriteRune(r)
			inSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}
