package feed

import (
	"strings"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

// PickImage selects the best image URL for a feed item, trying in order:
//  1. item.Image.URL
//  2. first image enclosure
//  3. <media:content url="…"> / <media:thumbnail url="…"> (via item.Extensions)
//  4. first <img src> inside Content or Description
//
// Returns "" if nothing is found.
func PickImage(it *gofeed.Item) string {
	if it == nil {
		return ""
	}
	if it.Image != nil && it.Image.URL != "" {
		return it.Image.URL
	}
	for _, e := range it.Enclosures {
		if e != nil && e.URL != "" && strings.HasPrefix(strings.ToLower(e.Type), "image/") {
			return e.URL
		}
	}
	if u := mediaExtensionURL(it); u != "" {
		return u
	}
	if u := firstImgSrc(it.Content); u != "" {
		return u
	}
	if u := firstImgSrc(it.Description); u != "" {
		return u
	}
	return ""
}

func mediaExtensionURL(it *gofeed.Item) string {
	media, ok := it.Extensions["media"]
	if !ok {
		return ""
	}
	for _, key := range []string{"content", "thumbnail"} {
		entries := media[key]
		for _, ext := range entries {
			if u := ext.Attrs["url"]; u != "" {
				return u
			}
		}
	}
	return ""
}

func firstImgSrc(body string) string {
	if body == "" {
		return ""
	}
	z := html.NewTokenizer(strings.NewReader(body))
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			if string(name) != "img" || !hasAttr {
				continue
			}
			for {
				k, v, more := z.TagAttr()
				if string(k) == "src" {
					src := strings.TrimSpace(string(v))
					if src != "" && !strings.HasPrefix(src, "data:") {
						return src
					}
				}
				if !more {
					break
				}
			}
		}
	}
}
