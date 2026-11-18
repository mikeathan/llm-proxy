// Package devicejoin is the one place where what the network knows of a device meets what the air says of it (ADR-0061). It holds no store and
// no key: it reads the network inventory, pulls the sensing stack's air report through the stack host, makes the vault's tag of each inventory
// hardware address under the purposes the stack itself asked the lan port for, and keeps only the facts whose tag is an inventory device's. A
// tag of anything else (a neighbour's device, a passer-by) matches nothing and is dropped. Either side may be off: the join is a column, never
// a condition, and "not heard" is never "not there".
package devicejoin

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/models"
)

const (
	// cacheFor is how long one join is kept: the Network page polls, the air changes by the cycle (minutes).
	cacheFor = time.Minute
	// airTimeout bounds one pull of the report; a slow stack is "could not read".
	airTimeout = 3 * time.Second
	// futureSlack is how far ahead of the clock a time from the stack may be before it is not believed.
	futureSlack = time.Minute
	// pageTagBytes is how much of a device's tag names the sensing stack's page about it: Radio's device page is addressed by the first four
	// bytes of the tag, in hex (stacks/radio shortID).
	pageTagBytes = 4
)

// Deps are what the join reads. Air is nil when no sensing stack is hosted.
type Deps struct {
	Inventory func() (models.DeviceInventory, error)
	Air       func(ctx context.Context) (stacksdk.AirReport, error)
	// Purposes are the purposes the stack asked the lan port for, newest first (stackport.AskedPurposes).
	Purposes func() []string
	// Tag is the vault's tag of a value under a purpose (stackport.Vault.Tag).
	Tag func(ctx context.Context, purpose string, value []byte) ([]byte, error)
	Now func() time.Time
	// OnError is told why the join could not be read, once per distinct cause until a read works again; nil tells no one.
	OnError func(error)
}

// Join joins the air to the inventory, and maps a stack's tag back to an inventory device.
type Join struct {
	deps Deps

	mu     sync.Mutex
	cached models.AirView
	at     time.Time
	last   string // the cause told last, so a repeating failure is told once
}

// New builds the join; nothing is read until View.
func New(deps Deps) *Join {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Join{deps: deps}
}

// View is the join now, read at most once a minute. It never fails: a failure is the AirUnreadable state.
func (j *Join) View(ctx context.Context) models.AirView {
	if j == nil {
		return models.AirView{State: models.AirOff}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	now := j.deps.Now()
	if !j.at.IsZero() && now.Sub(j.at) < cacheFor {
		return j.cached
	}
	j.cached, j.at = j.read(ctx, now), now
	return j.cached
}

func (j *Join) read(ctx context.Context, now time.Time) models.AirView {
	if j.deps.Air == nil {
		return models.AirView{State: models.AirOff}
	}
	actx, cancel := context.WithTimeout(ctx, airTimeout)
	defer cancel()
	rep, err := j.deps.Air(actx)
	switch {
	case stacksdk.CodeOf(err) == stacksdk.CodeUnavailable:
		j.told(nil)
		return models.AirView{State: models.AirOff}
	case err != nil:
		j.told(fmt.Errorf("the air report could not be read: %w", err))
		return models.AirView{State: models.AirUnreadable}
	}
	index, err := j.Index(ctx)
	if err != nil {
		j.told(fmt.Errorf("the inventory could not be joined to the air: %w", err))
		return models.AirView{State: models.AirUnreadable}
	}
	j.told(nil)
	v := models.AirView{State: models.AirNotListening, LastCycle: believed(rep.LastCycle, now), Bands: onlyFrom(rep.Bands, stacksdk.AirBands), ByMAC: map[string]models.DeviceAir{}}
	if rep.Listened {
		v.State = models.AirListening
	}
	for _, f := range rep.Devices[:min(len(rep.Devices), stacksdk.MaxAirFacts)] {
		if d, ok := index[string(f.Tag)]; ok {
			v.ByMAC[canonical(d.MAC)] = cleanFact(f, now)
		}
	}
	return v
}

// told reports a cause once until it changes; nil marks a read that worked. mu is held.
func (j *Join) told(err error) {
	if err == nil {
		j.last = ""
		return
	}
	if err.Error() == j.last {
		return
	}
	j.last = err.Error()
	if j.deps.OnError != nil {
		j.deps.OnError(err)
	}
}

// Index is every inventory device by the tag the stack knows it by, under each purpose the stack asked the lan port for. An inventory that
// cannot be read is an error; a device whose address does not parse is left out.
func (j *Join) Index(ctx context.Context) (map[string]models.NetworkDevice, error) {
	inv, err := j.deps.Inventory()
	if err != nil {
		return nil, err
	}
	out := map[string]models.NetworkDevice{}
	if j.deps.Purposes == nil || j.deps.Tag == nil {
		return out, nil
	}
	for _, purpose := range j.deps.Purposes() {
		for _, d := range inv.Devices {
			mac, err := net.ParseMAC(d.MAC)
			if err != nil || len(mac) != 6 {
				continue
			}
			tag, err := j.deps.Tag(ctx, purpose, mac)
			if err != nil {
				return nil, err
			}
			out[string(tag)] = d
		}
	}
	return out, nil
}

// cleanFact keeps the fixed words the SDK lists and drops any other, and a time from the future.
func cleanFact(f stacksdk.AirFact, now time.Time) models.DeviceAir {
	d := models.DeviceAir{Heard: believed(f.Heard, now), Moving: f.Moving, OnYourWiFi: f.OnYourWiFi, Page: pageOf(f.Tag)}
	d.Band, d.Ring, d.Kind = oneOf(f.Band, stacksdk.AirBands), oneOf(f.Ring, stacksdk.AirRings), oneOf(f.Kind, stacksdk.AirKinds)
	if d.Heard.IsZero() {
		d.Band, d.Ring, d.Moving = "", "", false // where it is and how it moves are only known while it is heard
	}
	return d
}

// pageOf is the id of the stack's page about the device of this tag: made here from the tag, so nothing the stack says becomes a link.
func pageOf(tag []byte) string {
	if len(tag) < pageTagBytes {
		return ""
	}
	return hex.EncodeToString(tag[:pageTagBytes])
}

func believed(t, now time.Time) time.Time {
	if t.After(now.Add(futureSlack)) {
		return time.Time{}
	}
	return t
}

func oneOf(v string, list []string) string {
	if slices.Contains(list, v) {
		return v
	}
	return ""
}

func onlyFrom(vs, list []string) []string {
	var out []string
	for _, v := range vs {
		if slices.Contains(list, v) && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// Of is what the air says of the device with this hardware address, when the join has it.
func Of(v models.AirView, mac string) (models.DeviceAir, bool) {
	d, ok := v.ByMAC[canonical(mac)]
	return d, ok
}

// canonical is a hardware address in one spelling (lower case, colons), empty when it is not one.
func canonical(s string) string {
	hw, err := net.ParseMAC(s)
	if err != nil {
		return ""
	}
	return strings.ToLower(hw.String())
}

// HeardRecently is how lately a device must have been heard for the new-device note to say where it was.
const HeardRecently = time.Hour

// NewDeviceNote is the fixed phrase the new-device alert gains (ADR-0061): where the air heard the new devices, by counts only. It is empty
// unless the sensor listened in the last day, so a quiet air is never told as "nobody is there".
func NewDeviceNote(v models.AirView, macs []string, now time.Time) string {
	if v.State != models.AirListening || len(macs) == 0 {
		return ""
	}
	var inside, outside int
	for _, m := range macs {
		d, ok := Of(v, m)
		if !ok || d.Heard.IsZero() || now.Sub(d.Heard) > HeardRecently {
			continue
		}
		switch d.Ring {
		case "room", "house":
			inside++
		case "nearby", "far":
			outside++
		}
	}
	return noteWords(len(macs), inside, outside)
}

func noteWords(total, inside, outside int) string {
	switch {
	case total == 1 && inside == 1:
		return "Radio heard it over the air inside the house."
	case total == 1 && outside == 1:
		return "Radio heard it over the air only from outside the house."
	case total == 1:
		return "Radio has not heard it over the air lately."
	case inside+outside == 0:
		return "Radio has not heard them over the air lately."
	}
	var parts []string
	if inside > 0 {
		parts = append(parts, plural(inside)+" inside the house")
	}
	if outside > 0 {
		parts = append(parts, plural(outside)+" only from outside the house")
	}
	return "Radio heard " + strings.Join(parts, " and ") + "."
}

func plural(n int) string {
	if n == 1 {
		return "1 of them"
	}
	return strconv.Itoa(n) + " of them"
}
