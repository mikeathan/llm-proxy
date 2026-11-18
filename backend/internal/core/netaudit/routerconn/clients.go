package routerconn

import (
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"llm-proxy/models"
)

// The mesh router's list of joined devices (`/tmp/clientlist.json` on an ASUS AiMesh router, Constitution I.15, ADR-0059): an object per box,
// keyed by the box's hardware address, holding an object per band, holding an object per device keyed by its hardware address with its
// address on this network and its signal. Measured on one router (2026-11-18): the boxes come in either order, a "wired_mac" object appears in
// some reads and not others, and each satellite's own backhaul radios are listed as clients of the main box. The list comes from the router, so
// it is read as untrusted: bounded, every value checked, and the addresses on this network dropped.
const (
	maxMeshBoxes   = 8
	maxMeshBands   = 16
	maxMeshClients = 1024
	minRSSI        = -120
)

// ErrBadClientList is a list that is not the shape this reader knows.
var ErrBadClientList = errors.New("the router's list of devices is not in the shape this program reads")

// bandKey is a wireless band of a box: 2G, 5G (also 5G1 and 5G2 on tri-band boxes) or 6G, and a guest network on it as "_1", "_2"...
var bandKey = regexp.MustCompile(`^(2G|5G[12]?|6G)(_[1-9])?$`)

type meshEntry struct {
	RSSI string `json:"rssi"`
}

// ParseClients reads one list. Keys that are not a band (such as "wired_mac") are skipped; a device that is a box's own radio (it shares its
// first five bytes with a box, as the backhaul radios of the measured router do) is dropped; the boxes are read in a fixed order.
func ParseClients(raw []byte) ([]models.MeshClient, error) {
	var boxes map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &boxes); err != nil {
		return nil, ErrBadClientList
	}
	if len(boxes) > maxMeshBoxes {
		return nil, ErrBadClientList
	}
	keys := make([]string, 0, len(boxes))
	prefixes := map[string]bool{}
	for k := range boxes {
		mac, ok := hardware(k)
		if !ok {
			return nil, ErrBadClientList
		}
		keys = append(keys, k)
		prefixes[mac[:14]] = true
	}
	sort.Strings(keys)
	var out []models.MeshClient
	for _, k := range keys {
		box, _ := hardware(k)
		got, err := boxClients(box, boxes[k], prefixes)
		if err != nil {
			return nil, err
		}
		if len(out)+len(got) > maxMeshClients {
			return nil, ErrBadClientList
		}
		out = append(out, got...)
	}
	return out, nil
}

// boxClients reads one box's bands.
func boxClients(box string, bands map[string]json.RawMessage, boxPrefixes map[string]bool) ([]models.MeshClient, error) {
	if len(bands) > maxMeshBands {
		return nil, ErrBadClientList
	}
	var out []models.MeshClient
	for name, rawBand := range bands {
		m := bandKey.FindStringSubmatch(name)
		if m == nil {
			continue // "wired_mac" and anything else that is not a wireless band
		}
		var devices map[string]meshEntry
		if err := json.Unmarshal(rawBand, &devices); err != nil || len(devices) > maxMeshClients {
			return nil, ErrBadClientList
		}
		for k, e := range devices {
			mac, ok := hardware(k)
			rssi, err := strconv.Atoi(e.RSSI)
			if !ok || err != nil || rssi < minRSSI || rssi > 0 {
				continue
			}
			if boxPrefixes[mac[:14]] {
				continue // a box's own radio: the mesh's backhaul, not a device
			}
			out = append(out, models.MeshClient{MAC: mac, Box: box, Band: m[1][:2], Guest: m[2] != "", RSSI: rssi})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	return out, nil
}

// hardware is a six-byte hardware address in lower case, colon separated.
func hardware(s string) (string, bool) {
	mac, err := net.ParseMAC(s)
	if err != nil || len(mac) != 6 {
		return "", false
	}
	return strings.ToLower(mac.String()), true
}
