// Package datausage reports what the service keeps under its data folder: one row per kind of store, with its size, so the data management
// page can show the whole and nothing hides (plan cross-cutting/data-management-page.md, phase D1). It only reads: it walks the folder's top
// level, measures each entry (folders through a cached, bounded walk) and names it from a table the service gives it. An entry no table names
// is still a row, under its own file name, marked as not described. Sizes are accounting, not a quota.
package datausage

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Kind names one kind of store: the top-level names it covers (an exact name, or a prefix ending in "*"), what it holds, and where it is
// removed. Owner is "core" or the id of the stack that keeps it.
type Kind struct {
	ID    string
	Title string
	Owner string
	Names []string
	// Clean says where it can be removed, in fixed words; empty when the page offers no way.
	Clean string
}

// Row is one kind of store as the page shows it. Entries are the top-level names it covers; Changed is the newest change among them.
type Row struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Owner     string    `json:"owner"`
	Entries   []string  `json:"entries"`
	Bytes     int64     `json:"bytes"`
	Changed   time.Time `json:"changed,omitzero"`
	Clean     string    `json:"clean,omitempty"`
	Described bool      `json:"described"`
	// CapBytes, Records, Oldest and KeepFor come from a stack's own report (stacksdk.StorageUse); zero when it gives none.
	CapBytes int64         `json:"cap_bytes,omitempty"`
	Records  int           `json:"records,omitempty"`
	Oldest   time.Time     `json:"oldest,omitzero"`
	KeepFor  time.Duration `json:"keep_for,omitempty"`
}

// Sizer measures a folder; sizewatch.Cache.Bytes is one.
type Sizer func(dir string, now time.Time) int64

// maxEntries bounds the top-level entries read, whatever the folder holds.
const maxEntries = 4096

// Measure reads the top level of root and returns one row per kind that has an entry, then one row per entry no kind names, largest
// first. A root that cannot be read gives no rows and the error.
func Measure(root string, kinds []Kind, size Sizer, now time.Time) ([]Row, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	rows := map[string]*Row{}
	var order []string
	for _, e := range entries[:min(len(entries), maxEntries)] {
		k, ok := kindOf(kinds, e.Name())
		if !ok {
			k = Kind{ID: "other:" + e.Name(), Title: e.Name()}
		}
		r, seen := rows[k.ID]
		if !seen {
			r = &Row{ID: k.ID, Title: k.Title, Owner: k.Owner, Clean: k.Clean, Described: ok}
			rows[k.ID] = r
			order = append(order, k.ID)
		}
		bytes, changed := entrySize(filepath.Join(root, e.Name()), e, size, now)
		r.Entries = append(r.Entries, e.Name())
		r.Bytes += bytes
		if changed.After(r.Changed) {
			r.Changed = changed
		}
	}
	out := make([]Row, 0, len(order))
	for _, id := range order {
		out = append(out, *rows[id])
	}
	slices.SortStableFunc(out, func(a, b Row) int {
		if a.Described != b.Described {
			if a.Described {
				return -1
			}
			return 1
		}
		return cmp.Compare(b.Bytes, a.Bytes)
	})
	return out, nil
}

// kindOf finds the kind that names a top-level entry.
func kindOf(kinds []Kind, name string) (Kind, bool) {
	for _, k := range kinds {
		for _, n := range k.Names {
			if n == name || (strings.HasSuffix(n, "*") && strings.HasPrefix(name, strings.TrimSuffix(n, "*"))) {
				return k, true
			}
		}
	}
	return Kind{}, false
}

// entrySize is a file's size, or a folder's through the sizer, with when it last changed. A symlink is counted as itself, never followed.
func entrySize(path string, e os.DirEntry, size Sizer, now time.Time) (int64, time.Time) {
	info, err := e.Info()
	if err != nil {
		return 0, time.Time{}
	}
	if e.IsDir() && size != nil {
		return size(path, now), info.ModTime()
	}
	return info.Size(), info.ModTime()
}

// Report is the whole page: the rows under the data folder and their total, the workspace folders (people's own work: size only, never a
// delete here), and the data folder's volume with its level from the host watch's rule (hostwatch.DiskLevel).
type Report struct {
	Root       string `json:"root"`
	Rows       []Row  `json:"rows"`
	Total      int64  `json:"total"`
	Workspaces *Row   `json:"workspaces,omitempty"`
	Free       uint64 `json:"free"`
	Size       uint64 `json:"size"`
	// Level is "ok", "low", "critical", or "unknown" when the volume could not be read.
	Level string `json:"level"`
}

// The levels of Report.Level.
const (
	LevelOK       = "ok"
	LevelLow      = "low"
	LevelCritical = "critical"
	LevelUnknown  = "unknown"
)

// Total sums the rows.
func Total(rows []Row) int64 {
	var t int64
	for _, r := range rows {
		t += r.Bytes
	}
	return t
}
