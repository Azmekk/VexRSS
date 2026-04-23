package server

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
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
		"dict":      dictFunc,
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

// dictFunc lets templates construct an ad-hoc map for the `{{template}}`
// directive, which only accepts a single data argument. Usage:
//
//	{{template "retention_form" (dict "RetentionDays" .X "Saved" false)}}
func dictFunc(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict: expected even number of args, got %d", len(pairs))
	}
	out := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		k, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key at index %d is not a string", i)
		}
		out[k] = pairs[i+1]
	}
	return out, nil
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

func ParseTemplates(fsys fs.FS, staticFS fs.FS) (*Templates, error) {
	funcs := Funcs()

	// Compute short content hashes of the volatile static assets and register
	// a `staticURL` template helper that appends `?v=<hash>`. Browsers cache
	// the path forever; the URL changes only when the file content changes,
	// so new deploys invalidate old caches without manual version bumps.
	versions, err := computeStaticVersions(staticFS, "app.css", "app.js")
	if err != nil {
		return nil, fmt.Errorf("compute static versions: %w", err)
	}
	funcs["staticURL"] = func(name string) string {
		if v, ok := versions[name]; ok {
			return "/static/" + name + "?v=" + v
		}
		return "/static/" + name
	}

	partialFiles := []string{
		"templates/partials/card.html",
		"templates/partials/cards.html",
		"templates/partials/source_row.html",
		"templates/partials/sources_list.html",
		"templates/partials/retention_form.html",
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

// computeStaticVersions reads each named file under the static FS and returns
// a {name: short-hex-hash} map. Used for cache-busting via `?v=<hash>` query
// strings on embedded CSS/JS assets.
func computeStaticVersions(staticFS fs.FS, names ...string) (map[string]string, error) {
	if staticFS == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(names))
	for _, n := range names {
		data, err := fs.ReadFile(staticFS, n)
		if err != nil {
			return nil, fmt.Errorf("read static %q: %w", n, err)
		}
		sum := sha256.Sum256(data)
		out[n] = hex.EncodeToString(sum[:])[:10]
	}
	return out, nil
}

// compile-time sanity check so we can use embed.FS without an import loop elsewhere
var _ fs.FS = (*embed.FS)(nil)
