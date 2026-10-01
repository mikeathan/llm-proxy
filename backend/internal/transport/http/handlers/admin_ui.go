package handlers

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const (
	adminMountPrefix = "/admin"
	adminIndexFile   = "index.html"
	// adminAPIDir is the admin API namespace below the mount; unknown API
	// paths must 404 instead of returning the SPA shell.
	adminAPIDir = "api/"
	// adminAssetsDir is Vite's build output directory for hashed bundles.
	adminAssetsDir = "assets/"
)

// serveAdminUI serves the embedded admin SPA rooted at fsys (plan D18).
// Existing files are served as-is. Any other path is a client-side route and
// gets index.html, so a hard refresh on a deep link works — except paths that
// can only be files (the API namespace, Vite's assets directory, and files at
// the root such as /admin/favicon.png): those 404, so a stale bundle or a bad
// API call fails loudly instead of receiving HTML.
func serveAdminUI(fsys fs.FS, w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, adminMountPrefix), "/")
	if p == "" {
		p = adminIndexFile
	}

	if _, err := fs.Stat(fsys, p); errors.Is(err, fs.ErrNotExist) {
		if !isClientRoute(p) {
			http.NotFound(w, r)
			return
		}
		p = adminIndexFile
	}

	// FileServer redirects /index.html to /, so the shell is requested as the root.
	r.URL.Path = "/"
	if p != adminIndexFile {
		r.URL.Path += p
	}
	http.FileServer(http.FS(fsys)).ServeHTTP(w, r)
}

// isClientRoute reports whether a missing path belongs to the SPA router.
// Nested route segments may carry an extension (a workspace file path such as
// workspaces/ws/files/notes.md), so only root-level names are treated as files.
func isClientRoute(p string) bool {
	if strings.HasPrefix(p, adminAPIDir) || strings.HasPrefix(p, adminAssetsDir) {
		return false
	}
	return strings.Contains(p, "/") || path.Ext(p) == ""
}
