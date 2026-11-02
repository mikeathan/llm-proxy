package hostwatch

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// maxKeyFiles bounds how many files one look inspects, whatever the globs match.
const maxKeyFiles = 256

// keyFiles counts the private key files that are open to other users. Only the mode is read (os.Stat, which follows a link,
// because a link's own mode is always 0777); a key under a folder this account may not enter is counted as unreadable, not
// as open. Public keys (.pub) are meant to be shared and are skipped.
func (h Hardener) keyFiles(out *Hardening) {
	seen := 0
	for _, g := range h.KeyGlobs {
		matches, _ := filepath.Glob(g) // only a malformed pattern errors, and the patterns are constants
		for _, p := range matches {
			if strings.HasSuffix(p, ".pub") {
				continue
			}
			if seen++; seen > maxKeyFiles {
				return
			}
			info, err := os.Stat(p)
			switch {
			case errors.Is(err, fs.ErrNotExist):
			case err != nil:
				out.Unreadable++
			case info.Mode().IsRegular() && info.Mode().Perm()&0o077 != 0:
				out.OpenKeyFiles++
			}
		}
	}
}
