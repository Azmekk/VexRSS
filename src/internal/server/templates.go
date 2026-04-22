package server

import (
	"embed"
	"fmt"
	"hash/fnv"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// Funcs are the template helpers available to every template.
func Funcs() template.FuncMap {
	return template.FuncMap{
		"timeAgo":   timeAgo,
		"hueFor":    hueFor,
		"hostOf":    hostOf,
		"trimBlurb": trimBlurb,
		"safeCSSURL": func(u string) template.CSS {
			// Only used for CSS url(...) values we set per-card. We sanitise by
			// stripping ) ( and backslashes, which are enough to break out of a
			// url() context. The image URL itself came from a remote feed, so we
			// also block obvious bad schemes.
			u = strings.TrimSpace(u)
			if u == "" {
				return template.CSS("none")
			}
			low := strings.ToLower(u)
			if strings.HasPrefix(low, "javascript:") || strings.HasPrefix(low, "data:text") {
				return template.CSS("none")
			}
			u = strings.NewReplacer("(", "", ")", "", "\\", "", "\"", "", "'", "").Replace(u)
			return template.CSS("url(\"" + u + "\")")
		},
	}
}

func timeAgo(t any) string {
	var ts time.Time
	switch v := t.(type) {
	case time.Time:
		ts = v
	case *time.Time:
		if v == nil {
			return ""
		}
		ts = *v
	default:
		return ""
	}
	if ts.IsZero() {
		return ""
	}
	d := time.Since(ts)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return ts.Format("Jan 2, 2006")
	}
}

// hueFor produces a deterministic 0–359 hue from a string, so sources and
// imageless cards get a stable, distinct color.
func hueFor(s string) int {
	h := fnv.New32a()
	_, _ = io.WriteString(h, s)
	return int(h.Sum32() % 360)
}

func hostOf(u string) string {
	i := strings.Index(u, "://")
	if i == -1 {
		return u
	}
	rest := u[i+3:]
	if j := strings.IndexByte(rest, '/'); j != -1 {
		rest = rest[:j]
	}
	return strings.TrimPrefix(rest, "www.")
}

func trimBlurb(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndexByte(cut, ' '); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "…"
}

// Templates wraps parsed html/template sets. Each full page gets its own set
// so that a {{define "content"}} in one page doesn't collide with another's.
// Partials are parsed standalone for htmx swaps.
type Templates struct {
	pages    map[string]*template.Template
	partials *template.Template
}

func ParseTemplates(fsys fs.FS) (*Templates, error) {
	funcs := Funcs()

	partialFiles := []string{
		"templates/partials/card.html",
		"templates/partials/cards.html",
		"templates/partials/source_row.html",
		"templates/partials/sources_list.html",
	}

	pages := map[string]*template.Template{}
	for _, pg := range []struct{ name, file string }{
		{"index", "templates/index.html"},
		{"settings", "templates/settings.html"},
	} {
		files := append([]string{"templates/layout.html", pg.file}, partialFiles...)
		t, err := template.New(pg.name).Funcs(funcs).ParseFS(fsys, files...)
		if err != nil {
			return nil, fmt.Errorf("parse page %s: %w", pg.name, err)
		}
		pages[pg.name] = t
	}

	partials, err := template.New("partials").Funcs(funcs).ParseFS(fsys, partialFiles...)
	if err != nil {
		return nil, err
	}
	return &Templates{pages: pages, partials: partials}, nil
}

// RenderPage renders the full-page template named name.
func (t *Templates) RenderPage(w http.ResponseWriter, name string, data any) error {
	p, ok := t.pages[name]
	if !ok {
		return fmt.Errorf("unknown page template: %s", name)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return p.ExecuteTemplate(w, name, data)
}

// RenderPartial renders a standalone partial (for htmx swaps).
func (t *Templates) RenderPartial(w http.ResponseWriter, name string, data any) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return t.partials.ExecuteTemplate(w, name, data)
}

// compile-time sanity check so we can use embed.FS without an import loop elsewhere
var _ fs.FS = (*embed.FS)(nil)
