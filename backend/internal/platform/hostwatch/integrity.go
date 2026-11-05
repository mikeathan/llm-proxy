package hostwatch

import (
	"bufio"
	"cmp"
	"context"
	"crypto/md5" // #nosec G501 -- dpkg's own checksum format; used to compare with what a package shipped, never to protect anything
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Limits of one pass.
const (
	// MaxIntegrityPaths is how many changed paths a result lists; the count is always the whole number.
	MaxIntegrityPaths = 10
	// MaxUnreadablePlaces is how many directories a result names as holding the files that could not be read.
	MaxUnreadablePlaces = 3
	// placeDepth is how many path components name such a directory ("/usr/lib/modules").
	placeDepth = 3
	// MaxIntegrityFileBytes is the largest file that is hashed. A larger one is counted as not looked at.
	MaxIntegrityFileBytes = 512 << 20
	md5Hex                = 32
)

// DefaultIntegrityPrefixes are the program directories whose packaged files are compared, relative to the root as dpkg lists
// them. Configuration under /etc, documentation and data are left out: people edit those and nothing in them runs.
var DefaultIntegrityPrefixes = []string{"usr/bin/", "usr/sbin/", "usr/lib/", "usr/libexec/", "bin/", "sbin/", "lib/", "lib64/"}

// safeListedPath is what a path must look like to be named in a result, so a file name cannot carry more than a name.
var safeListedPath = regexp.MustCompile(`^/[A-Za-z0-9._+@/-]{1,200}$`)

// Integrity compares the program files that packages installed with the checksums those packages shipped, from the
// "<package>.md5sums" files dpkg keeps (readable by every account). It starts no program and keeps no file text: the result is
// a few numbers and at most MaxIntegrityPaths plain paths.
type Integrity struct {
	// InfoDir is "/var/lib/dpkg/info" and Root "/" on a real computer.
	InfoDir string
	Root    string
	// Prefixes are the directories compared; DefaultIntegrityPrefixes when empty.
	Prefixes []string
	// afterPass runs between the first look and the re-check; a test uses it to stand in for an update that finishes meanwhile.
	afterPass func()
}

// IntegrityResult is what one pass found.
type IntegrityResult struct {
	// Complete is true only when every file in scope was looked at. A pass that was cut short, or whose list of packages could
	// not be read, is not complete, and the caller must not read it as "nothing changed".
	Complete bool
	// Checked is how many files were compared, Modified how many differ from their package's checksum, and Unreadable how many
	// could not be read (or were too large) and so were not compared.
	Checked    int
	Modified   int
	Unreadable int
	// UnreadableIn names up to MaxUnreadablePlaces directories holding most of those files, each "/usr/lib/modules (120)", the
	// largest first; UnreadableWhy is the operating system's reason for the first one ("permission denied"). Both are empty
	// when every file could be read.
	UnreadableIn  []string
	UnreadableWhy string
	// Paths names up to MaxIntegrityPaths of the changed files, sorted, each a plain absolute path.
	Paths []string
}

type fileClaim struct{ pkg, rel string }

// Check takes one pass. ctx bounds it: when it ends the result so far is returned with Complete false.
func (i Integrity) Check(ctx context.Context) IntegrityResult {
	prefixes := i.Prefixes
	if len(prefixes) == 0 {
		prefixes = DefaultIntegrityPrefixes
	}
	lists, err := filepath.Glob(filepath.Join(i.InfoDir, "*.md5sums"))
	if err != nil || len(lists) == 0 {
		return IntegrityResult{}
	}
	var res IntegrityResult
	var suspects []fileClaim
	complete := true
	places := map[string]int{}
	for _, list := range lists {
		if !i.scanList(ctx, list, prefixes, &res, &suspects, places) {
			complete = false
			break
		}
	}
	if i.afterPass != nil {
		i.afterPass()
	}
	res.Complete = complete
	res.UnreadableIn = topPlaces(places)
	res.Modified, res.Paths = i.confirm(suspects)
	return res
}

// scanList compares the files one package's list names, adding to res and suspects. It returns false when ctx ended first.
func (i Integrity) scanList(ctx context.Context, list string, prefixes []string, res *IntegrityResult, suspects *[]fileClaim, places map[string]int) bool {
	pkg := strings.TrimSuffix(filepath.Base(list), ".md5sums")
	skip := i.conffiles(pkg)
	for _, c := range readClaims(list) {
		if ctx.Err() != nil {
			return false
		}
		if !inScope(c.rel, prefixes) || skip[c.rel] {
			continue
		}
		v, why := i.compare(c.rel, c.sum)
		switch v {
		case verdictSame:
			res.Checked++
		case verdictDiffers:
			res.Checked++
			*suspects = append(*suspects, fileClaim{pkg: pkg, rel: c.rel})
		case verdictUnreadable:
			res.Unreadable++
			places[placeOf(c.rel)]++
			if res.UnreadableWhy == "" {
				res.UnreadableWhy = whyText(why)
			}
		}
	}
	return true
}

type verdict int

const (
	verdictSkip verdict = iota // not there, not a plain file, or nothing to compare
	verdictSame
	verdictDiffers
	verdictUnreadable
)

func (i Integrity) compare(rel, want string) (verdict, error) {
	full := filepath.Join(i.Root, rel)
	info, err := os.Lstat(full)
	switch {
	case err != nil && os.IsNotExist(err):
		return verdictSkip, nil
	case err != nil:
		return verdictUnreadable, err
	case !info.Mode().IsRegular():
		return verdictSkip, nil
	case info.Size() > MaxIntegrityFileBytes:
		return verdictUnreadable, errTooLarge
	}
	got, err := hashFileMD5(full)
	switch {
	case err != nil:
		return verdictUnreadable, err
	case got == want:
		return verdictSame, nil
	}
	return verdictDiffers, nil
}

var errTooLarge = errors.New("too large to compare")

// whyText is the operating system's reason without the path, which the result names separately.
func whyText(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return strings.ToLower(err.Error())
}

// placeOf is the directory a package-relative path lies in, cut to placeDepth components, as an absolute path; a name that is not
// a plain safe path is "other places", so a package cannot put text of its own in a result.
func placeOf(rel string) string {
	dir := path.Dir(rel)
	if parts := strings.Split(dir, "/"); len(parts) > placeDepth {
		dir = strings.Join(parts[:placeDepth], "/")
	}
	if p := "/" + dir; safeListedPath.MatchString(p) {
		return p
	}
	return "other places"
}

// topPlaces names the directories with the most unreadable files, largest first.
func topPlaces(places map[string]int) []string {
	names := make([]string, 0, len(places))
	for n := range places {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int {
		if c := cmp.Compare(places[b], places[a]); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	if len(names) > MaxUnreadablePlaces {
		names = names[:MaxUnreadablePlaces]
	}
	out := make([]string, len(names))
	for k, n := range names {
		out[k] = fmt.Sprintf("%s (%d)", n, places[n])
	}
	return out
}

// confirm looks again at each file that differed, against its package's list as it is now. A package update that finished
// during the pass rewrote the list and the file together, so a file that now matches is not a change.
func (i Integrity) confirm(suspects []fileClaim) (int, []string) {
	var paths []string
	modified := 0
	for _, s := range suspects {
		want, ok := latestSum(filepath.Join(i.InfoDir, s.pkg+".md5sums"), s.rel)
		if !ok {
			continue
		}
		if v, _ := i.compare(s.rel, want); v != verdictDiffers {
			continue
		}
		modified++
		if p := "/" + s.rel; safeListedPath.MatchString(p) {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	if len(paths) > MaxIntegrityPaths {
		paths = paths[:MaxIntegrityPaths]
	}
	return modified, paths
}

// conffiles are the package's configuration files, which dpkg expects people to edit.
func (i Integrity) conffiles(pkg string) map[string]bool {
	out := map[string]bool{}
	f, err := os.Open(filepath.Join(i.InfoDir, pkg+".conffiles"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) > 0 {
			out[strings.TrimPrefix(fields[0], "/")] = true
		}
	}
	return out
}

type claim struct{ sum, rel string }

// readClaims reads the lines of one checksum list that are a checksum and a plain relative path.
func readClaims(list string) []claim {
	f, err := os.Open(list)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []claim
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if c, ok := parseClaim(sc.Text()); ok {
			out = append(out, c)
		}
	}
	return out
}

func parseClaim(line string) (claim, bool) {
	sum, rel, ok := strings.Cut(line, "  ")
	if !ok || len(sum) != md5Hex || !isHex(sum) || rel == "" || strings.HasPrefix(rel, "/") || path.Clean(rel) != rel || strings.Contains(rel, "..") {
		return claim{}, false
	}
	return claim{sum: sum, rel: rel}, true
}

func isHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}

func inScope(rel string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}

// latestSum is the checksum a package's list gives for a path now.
func latestSum(list, rel string) (string, bool) {
	var sum string
	found := false
	for _, c := range readClaims(list) {
		if c.rel == rel {
			sum, found = c.sum, true
		}
	}
	return sum, found
}

func hashFileMD5(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New() // #nosec G401 -- see the import
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
