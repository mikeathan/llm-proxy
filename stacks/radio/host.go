package radio

import (
	"io/fs"
	"path"
	"time"
)

// This computer's own Wi-Fi: the built-in card that carries the computer's connection is heard over the air like any other device, always here
// and the strongest of all, and the network inventory never lists it (a computer does not see its own address in its neighbour table). Its
// address is read from the system's own records, compared as a tag like the inventory's, and never stored or shown.

// HostRadio is one of this computer's wireless interfaces: its name and its hardware address.
type HostRadio struct {
	Iface string
	MAC   MAC
}

// HostRadios lists this computer's wireless interfaces that have an address, reading sys (the /sys tree) and nothing else. A wired port is
// never heard over the air, so it is left out. A computer without /sys gives none.
func HostRadios(sys fs.FS) []HostRadio {
	entries, err := fs.ReadDir(sys, "class/net")
	if err != nil {
		return nil
	}
	var out []HostRadio
	for _, e := range entries {
		name := e.Name()
		dir, ok := deviceDir(sys, "net", name)
		if !ok || !token.MatchString(name) {
			continue
		}
		if !exists(sys, path.Join(dir, "phy80211")) && !exists(sys, path.Join(dir, "wireless")) {
			continue
		}
		mac, err := ParseMAC(readTrim(sys, path.Join(dir, "address")))
		if err != nil || mac == (MAC{}) {
			continue
		}
		out = append(out, HostRadio{Iface: name, MAC: mac})
	}
	return out
}

// SetHost gives the recorder this computer's wireless interfaces; the LAN index then marks each as this computer.
func (r *Recorder) SetHost(h []HostRadio) {
	r.lmu.Lock()
	defer r.lmu.Unlock()
	r.host, r.lanIndex, r.lanAt = h, nil, time.Time{}
}
