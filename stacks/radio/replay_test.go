package radio

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// readPcap returns the frames of a classic little-endian pcap file (link type radiotap).
func readPcap(t *testing.T, path string) [][]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 24 || binary.LittleEndian.Uint32(b) != 0xa1b2c3d4 && binary.LittleEndian.Uint32(b) != 0xa1b23c4d {
		t.Skipf("%s: not a little-endian pcap", path)
	}
	var out [][]byte
	for off := 24; off+16 <= len(b); {
		n := int(binary.LittleEndian.Uint32(b[off+8:]))
		off += 16
		if n < 0 || off+n > len(b) {
			break
		}
		out = append(out, b[off:off+n])
		off += n
	}
	return out
}

// TestReplayOwnerCaptures parses the owner's real captures and prints the counts to compare with `tcpdump -nn -e -r FILE`. The captures hold
// neighbours' addresses and are never in the repo: set RADIO_PCAP_DIR to a folder of them to run this.
func TestReplayOwnerCaptures(t *testing.T) {
	dir := os.Getenv("RADIO_PCAP_DIR")
	if dir == "" {
		t.Skip("RADIO_PCAP_DIR not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.pcap"))
	for _, p := range files {
		counts := map[FrameKind]int{}
		failed, secs := 0, map[SecMode]int{}
		for _, fr := range readPcap(t, p) {
			f, err := Parse(fr)
			if err != nil {
				failed++
				continue
			}
			counts[f.Kind]++
			if f.Kind == KindBeacon {
				secs[f.Security.Mode]++
			}
		}
		t.Logf("%s: failed=%d %v beacon-security=%v", filepath.Base(p), failed, counts, secs)
	}
}
