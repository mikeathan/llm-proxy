package radio

import (
	"context"
	"path"
	"time"

	"llm-proxy-stacks/sdk"
)

const (
	probeAction = "radio_probe"
	probeDoc    = "probe"
)

// ProbeRead is the last read of what the computer's wireless devices can do, kept with the day it was made.
type ProbeRead struct {
	At   time.Time `json:"at"`
	Phys []PhyCaps `json:"phys"`
}

// probeRequest is the one request a probe is: no parameters. It heads the list a session approves, so the run reads what the adapter can do
// under the same approval as the windows, with no grant path of its own.
func probeRequest() ListenRequest { return ListenRequest{Action: probeAction, Params: []byte(`{}`)} }

// readProbe asks the helper what the adapters can do and keeps the parsed answer. A refusal is returned (the run ends: no session approved it);
// any other failure is not: a probe that cannot be read leaves the page saying "not read yet", and the run goes on listening.
func (s *Stack) readProbe(ctx context.Context, priv stacksdk.Privileged, store *Store) error {
	res, err := priv.Run(ctx, stacksdk.PrivilegedRequest{Action: probeAction, Params: probeRequest().Params})
	if err != nil {
		if stacksdk.CodeOf(err) == stacksdk.CodeDenied {
			return err
		}
		return nil
	}
	read := ProbeRead{At: s.now(), Phys: ParseProbe(res.Data)}
	if len(read.Phys) == 0 {
		return nil
	}
	_ = store.PutDoc(ctx, probeDoc, read, read.At)
	return nil
}

// phyOf is the wireless device name ("phy1") behind a network interface, from the system's own records.
func (s *Stack) phyOf(iface string) string {
	return readTrim(s.sys, path.Join("class", "net", iface, "phy80211", "name"))
}
