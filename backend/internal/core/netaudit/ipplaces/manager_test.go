package ipplaces

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func gz(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	return b.Bytes()
}

func serving(files map[string][]byte, asked *[]string) Opener {
	return func(_ context.Context, url string) (*http.Response, error) {
		*asked = append(*asked, url[strings.LastIndex(url, "/")+1:])
		body, ok := files[url[strings.LastIndex(url, "/")+1:]]
		if !ok {
			return nil, errors.New("server returned unexpected status: 404 Not Found")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, nil
	}
}

var nov = time.Date(2026, 11, 31-1, 12, 0, 0, 0, time.UTC)

func TestDownloadKeepsTheIndexOnDiskAndLooksAddressesUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "places.bin")
	var asked []string
	m, err := NewManager(Deps{Path: path, Now: func() time.Time { return nov }, Open: serving(map[string][]byte{"dbip-city-lite-2026-11.csv.gz": gz(t, sample)}, &asked)})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Place(netip.MustParseAddr("1.0.0.1")); ok || m.Status().Ranges != 0 {
		t.Fatal("nothing is held before a download")
	}
	if err := m.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p, ok := m.Place(netip.MustParseAddr("1.1.0.9")); !ok || p.City != "Dublin" {
		t.Errorf("got %+v %v", p, ok)
	}
	if st := m.Status(); st.Ranges == 0 || !st.LoadedAt.Equal(nov) {
		t.Errorf("status %+v", st)
	}
	// A new manager on the same file reads it back: it is what survives a restart.
	again, err := NewManager(Deps{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := again.Place(netip.MustParseAddr("1.0.3.1")); !ok || p.City != "Wenquan" {
		t.Errorf("after a restart: %+v %v", p, ok)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("a temporary file was left: %v", entries)
	}
}

func TestDownloadFallsBackToLastMonthAndNamesTheMonthFromTheFirst(t *testing.T) {
	var asked []string
	m, _ := NewManager(Deps{Path: filepath.Join(t.TempDir(), "p.bin"), Now: func() time.Time { return time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC) }, Open: serving(map[string][]byte{"dbip-city-lite-2026-02.csv.gz": gz(t, sample)}, &asked)})
	if err := m.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, ",") != "dbip-city-lite-2026-03.csv.gz,dbip-city-lite-2026-02.csv.gz" {
		t.Errorf("asked %v", asked)
	}
}

func TestAFailedDownloadKeepsWhatWasHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.bin")
	var asked []string
	files := map[string][]byte{"dbip-city-lite-2026-11.csv.gz": gz(t, sample)}
	m, _ := NewManager(Deps{Path: path, Now: func() time.Time { return nov }, Open: serving(files, &asked)})
	if err := m.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"not gzip": []byte("<html>"), "no row": gz(t, "junk\n"), "none": nil} {
		files = map[string][]byte{"dbip-city-lite-2026-11.csv.gz": body}
		if name == "none" {
			files = map[string][]byte{}
		}
		m.deps.Open = serving(files, &asked)
		if err := m.Download(context.Background()); err == nil {
			t.Errorf("%s: want an error", name)
		}
		if p, ok := m.Place(netip.MustParseAddr("1.1.0.9")); !ok || p.City != "Dublin" {
			t.Errorf("%s: the held index was lost: %+v %v", name, p, ok)
		}
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("a temporary file was left: %v", entries)
	}
}

func TestRemoveForgetsTheIndexAndTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.bin")
	var asked []string
	m, _ := NewManager(Deps{Path: path, Now: func() time.Time { return nov }, Open: serving(map[string][]byte{"dbip-city-lite-2026-11.csv.gz": gz(t, sample)}, &asked)})
	_ = m.Download(context.Background())
	if err := m.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil || m.Status().Ranges != 0 {
		t.Error("the index must be gone")
	}
	if err := m.Remove(); err != nil {
		t.Errorf("removing nothing is not an error: %v", err)
	}
}

func TestAFileThatIsNotAnIndexStartsEmptyAndIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.bin")
	_ = os.WriteFile(path, []byte("garbage garbage garbage garbage garbage"), 0o600)
	m, err := NewManager(Deps{Path: path})
	if err == nil || m == nil {
		t.Fatalf("want a usable empty manager and an error, got %v", err)
	}
}
