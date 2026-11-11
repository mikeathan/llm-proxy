package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/ipowners"
	"llm-proxy/internal/core/netaudit/ipplaces"
)

type memOwnersStore struct {
	data []byte
	at   time.Time
}

func (s *memOwnersStore) Read() ([]byte, time.Time, error) { return s.data, s.at, nil }
func (s *memOwnersStore) Write(b []byte, at time.Time) error {
	s.data, s.at = b, at
	return nil
}
func (s *memOwnersStore) Delete() error { s.data = nil; return nil }

func gzipped(text string) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(text))
	_ = w.Close()
	return b.Bytes()
}

func fromBody(body []byte) ipowners.Opener {
	return func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	}
}

func TestAnOwnerSaysWhereItIsUsedOnlyOnceTheTableOfCitiesIsHeld(t *testing.T) {
	owners, _ := ipowners.NewManager(ipowners.Deps{Store: &memOwnersStore{}, Open: fromBody(gzipped("8.8.8.0\t8.8.8.255\t15169\tUS\tGOOGLE\n"))})
	if err := owners.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	places, _ := ipplaces.NewManager(ipplaces.Deps{Path: filepath.Join(t.TempDir(), "places.bin"), Open: fromBody(gzipped("8.8.8.0,8.8.8.255,NA,US,California,\"Mountain View\",37.4,-122.1\n"))})
	n := &netauditServices{DestinationOwners: owners, DestinationPlaces: places}
	addr := netip.MustParseAddr("8.8.8.8")

	o, ok := n.currentOwners()(addr)
	if !ok || o.ASN != 15169 || o.City != "" {
		t.Fatalf("before the cities: %+v %v", o, ok)
	}
	if err := places.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	o, ok = n.currentOwners()(addr)
	if !ok || o.City != "Mountain View" || o.Region != "California" || o.Country != "US" || o.PlaceCountry != "US" || o.Point == nil || o.Point.Lat != 37.4 || o.Point.Lon != -122.1 {
		t.Errorf("with the cities: %+v %v", o, ok)
	}
	if _, ok := n.currentOwners()(netip.MustParseAddr("9.9.9.9")); ok {
		t.Error("an address with no owner must stay unowned even if a city table could place it")
	}
	if !strings.Contains(ipplaces.Attribution, "DB-IP") {
		t.Error("the licence's credit must be kept")
	}
}
