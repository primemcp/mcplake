package controlplane

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// webuiDistFS embeds the admin web UI's built static assets. It is a
// placeholder (see webui/dist/index.html) until #78 (frontend scaffold)
// gives `make ui-build` something real to produce; `go:embed` requires the
// directory to exist with at least one file, so the placeholder keeps `go
// build`/`go test` working without a frontend toolchain installed.
//
//go:embed all:webui/dist
var webuiDistFS embed.FS

// WebUIAssets is webuiDistFS rooted at webui/dist, so index.html and every
// other asset are addressed without a "webui/dist/" prefix — the shape
// RegisterUIRoutes expects.
var WebUIAssets = mustSubFS(webuiDistFS, "webui/dist")

func mustSubFS(embedded embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(embedded, dir)
	if err != nil {
		panic(fmt.Sprintf("controlplane: sub embedded FS %q: %v", dir, err))
	}
	return sub
}

// RegisterUIRoutes serves the admin web UI (assets, typically WebUIAssets)
// from engine: a real file under assets is served directly; any other
// unmatched path is served index.html, so client-side navigation and deep
// links resolve (the frontend has no server-side router of its own — see
// the Admin UI design doc). Paths under "/admin" are left untouched: an
// unmatched "/admin/..." route still 404s exactly as it did before
// RegisterUIRoutes was added, since that's the JSON API's own namespace.
func RegisterUIRoutes(engine *gin.Engine, assets fs.FS) {
	fileServer := http.FileServer(http.FS(assets))

	serveIndex := func(c *gin.Context) {
		data, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	}

	engine.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if p == "/admin" || strings.HasPrefix(p, "/admin/") {
			c.Status(http.StatusNotFound)
			return
		}

		// http.FileServer redirects any request whose final path segment
		// is "index.html" to "./" (its directory-index convention) — which
		// loops here, since every unmatched path is served that same file.
		// serveIndex bypasses FileServer entirely for that file; every
		// other real asset still goes through FileServer for correct
		// content-type/range/caching behavior.
		clean := strings.TrimPrefix(path.Clean(p), "/")
		if clean != "" && clean != "index.html" {
			if f, err := assets.Open(clean); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}

		serveIndex(c)
	})
}
