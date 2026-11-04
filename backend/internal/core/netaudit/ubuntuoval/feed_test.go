package ubuntuoval

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"llm-proxy/models"
)

type memStore struct {
	idx     models.UbuntuIndex
	writes  int
	deleted bool
	err     error
}

func (m *memStore) Read() (models.UbuntuIndex, error) { return m.idx, nil }
func (m *memStore) Write(i models.UbuntuIndex) error {
	if m.err != nil {
		return m.err
	}
	m.idx, m.writes = i, m.writes+1
	return nil
}
func (m *memStore) Delete() error { m.deleted, m.idx = true, models.UbuntuIndex{}; return nil }

func okOpener(body []byte, finalURL string) Opener {
	return func(ctx context.Context, u string) (*http.Response, error) {
		fu, _ := url.Parse(finalURL)
		if finalURL == "" {
			fu, _ = url.Parse(u)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: &http.Request{URL: fu}}, nil
	}
}

func newManager(store Store, open Opener, release string) *Manager {
	m := New(store, open, func() string { return release })
	m.now = func() time.Time { return time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) }
	m.decode = func(r io.Reader) io.Reader { return r } // the bodies below are plain XML; bzip2 has its own test
	return m
}

func TestManager_UpdateFetchesTheOneFixedAddressForTheHostsReleaseAndKeepsTheIndex(t *testing.T) {
	store := &memStore{}
	var asked []string
	open := func(ctx context.Context, u string) (*http.Response, error) {
		asked = append(asked, u)
		return okOpener([]byte(sample), "")(ctx, u)
	}
	m := newManager(store, open, "noble")
	if err := m.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != "https://security-metadata.canonical.com/oval/com.ubuntu.noble.usn.oval.xml.bz2" {
		t.Errorf("asked = %v", asked)
	}
	st := m.Status()
	if store.writes != 1 || st.Notices != 2 || st.Release != "noble" || !st.ImportedAt.Equal(time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)) || st.Running {
		t.Errorf("writes %d, status %+v", store.writes, st)
	}
	if len(m.Index().Packages["openssl"]) != 1 {
		t.Error("the index is not available")
	}
}

func TestManager_APlainReleaseNameIsTheOnlyThingThatReachesTheAddress(t *testing.T) {
	for _, release := range []string{"", "../etc", "noble/../x", "NOBLE", "no ble", "a", strings.Repeat("a", 40)} {
		called := false
		m := newManager(&memStore{}, func(context.Context, string) (*http.Response, error) { called = true; return nil, errors.New("no") }, release)
		if err := m.Update(context.Background()); !errors.Is(err, ErrNotUbuntu) || called {
			t.Errorf("release %q: err %v, asked %v", release, err, called)
		}
	}
}

func TestManager_AnAnswerFromAnotherHostOrAFailureOrTooMuchLeavesWhatWasHeld(t *testing.T) {
	store := &memStore{idx: models.UbuntuIndex{Release: "noble", Notices: map[string]models.UbuntuNotice{"USN-1-1": {}}}}
	cases := map[string]Opener{
		"redirected":  okOpener([]byte(sample), "https://evil.example/x"),
		"downgraded":  okOpener([]byte(sample), "http://security-metadata.canonical.com/x"),
		"not xml":     okOpener([]byte("<html>nope"), ""),
		"open failed": func(context.Context, string) (*http.Response, error) { return nil, errors.New("down") },
		"status": func(context.Context, string) (*http.Response, error) {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(sample)), Request: &http.Request{URL: mustURL("https://security-metadata.canonical.com/x")}}, nil
		},
	}
	for name, open := range cases {
		m := newManager(store, open, "noble")
		if err := m.Update(context.Background()); err == nil {
			t.Errorf("%s: no error", name)
		}
		if store.writes != 0 || len(m.Index().Notices) != 1 {
			t.Errorf("%s: the data that was held changed (writes %d)", name, store.writes)
		}
		if m.Status().Reason == "" {
			t.Errorf("%s: no reason given", name)
		}
	}
}

func mustURL(s string) *url.URL { u, _ := url.Parse(s); return u }

func TestManager_AFileLargerThanTheCapIsRefusedBeforeItIsRead(t *testing.T) {
	m := newManager(&memStore{}, func(context.Context, string) (*http.Response, error) {
		// The body is a good file: only the announced size can turn it away.
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(sample)), ContentLength: MaxCompressedBytes + 1, Request: &http.Request{URL: mustURL("https://security-metadata.canonical.com/x")}}, nil
	}, "noble")
	if err := m.Update(context.Background()); err == nil {
		t.Error("an oversize file was accepted")
	}
}

func TestManager_OnlyOneUpdateRunsAtATimeAndAFailedSaveKeepsTheOldIndex(t *testing.T) {
	var running atomic.Int32
	release := make(chan struct{})
	open := func(ctx context.Context, u string) (*http.Response, error) {
		running.Add(1)
		select {
		case <-release:
		case <-ctx.Done(): // a second update that was wrongly let in gives up here, so the test fails and does not hang
			return nil, ctx.Err()
		}
		return okOpener([]byte(sample), "")(ctx, u)
	}
	m := newManager(&memStore{}, open, "noble")
	done := make(chan error, 1)
	go func() { done <- m.Update(context.Background()) }()
	for running.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	second, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := m.Update(second); !errors.Is(err, ErrBusy) {
		t.Errorf("second update: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	failing := &memStore{err: errors.New("disk full")}
	m2 := newManager(failing, okOpener([]byte(sample), ""), "noble")
	if err := m2.Update(context.Background()); err == nil || len(m2.Index().Notices) != 0 {
		t.Errorf("a failed save changed the held data: %v", err)
	}
}

func TestManager_ClearDeletesTheDataAndIsSafeWhenThereIsNone(t *testing.T) {
	store := &memStore{}
	m := newManager(store, okOpener([]byte(sample), ""), "noble")
	if err := m.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.Clear(); err != nil || !store.deleted || len(m.Index().Notices) != 0 || m.Status().Notices != 0 {
		t.Errorf("clear: %v deleted=%v status %+v", err, store.deleted, m.Status())
	}
	if err := m.Clear(); err != nil {
		t.Error(err)
	}
}

func TestManager_StatusSaysWhetherThisComputerCanUseTheData(t *testing.T) {
	if st := newManager(&memStore{}, nil, "noble").Status(); !st.Supported || st.HostRelease != "noble" {
		t.Errorf("status = %+v", st)
	}
	if st := newManager(&memStore{}, nil, "").Status(); st.Supported {
		t.Errorf("a computer that is not Ubuntu cannot use it: %+v", st)
	}
}

func TestManager_TheDataIsSharedWithNoCallerThatCouldChangeIt(t *testing.T) {
	m := newManager(&memStore{}, okOpener([]byte(sample), ""), "noble")
	_ = m.Update(context.Background())
	a := m.Index()
	a.Packages["openssl"][0].Fixed = "tampered"
	if m.Index().Packages["openssl"][0].Fixed == "tampered" {
		t.Error("a caller changed the held index")
	}
}

func TestManager_ReadsTheRealBzip2Stream(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample.usn.oval.xml.bz2")
	if err != nil {
		t.Fatal(err)
	}
	m := New(&memStore{}, okOpener(raw, ""), func() string { return "noble" })
	if err := m.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := m.Status(); st.Notices != 2 || st.Packages == 0 {
		t.Errorf("status = %+v", st)
	}
}
