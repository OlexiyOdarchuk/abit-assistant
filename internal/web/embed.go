package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// distFS holds the built Svelte frontend, embedded into the binary so the
// whole app ships as one executable. `all:` keeps files vite may name with a
// leading underscore/dot.
//
//go:embed all:dist
var distFS embed.FS

// staticHandler serves the embedded frontend with SPA fallback: a request for
// a path that isn't a real asset returns index.html so client-side routing
// works on deep links / refreshes.
func staticHandler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist is embedded at compile time; a failure here is a build bug.
		panic("web: embedded dist subtree: " + err.Error())
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			// Not a real asset → hand the SPA its entry point.
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}
