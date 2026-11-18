package radio

import "math"

// DeviceIdent is what the page needs to name a device and say why it thinks what it thinks (ADR-0058). It is worked out when the page asks,
// from the stored rows, the run's memory and the network inventory; nothing here is written to the disk.
type DeviceIdent struct {
	Kind       DeviceKind `json:"kind"`
	KindConf   int        `json:"kind_confidence"`
	KindSource KindSource `json:"kind_source"`
	// Brand is who made it: the WPS manufacturer, else the maker of its address block, else a vendor element's company (BrandHint).
	Brand     string `json:"brand,omitempty"`
	BrandHint bool   `json:"brand_hint,omitempty"`
	Model     string `json:"model,omitempty"`
	// LANName is the name the operator gave it on the Network page, when the network inventory has its address; RouterRadio says its address sits
	// next to the gateway's, where a router's Wi-Fi radios are.
	LANName     string `json:"lan_name,omitempty"`
	RouterRadio bool   `json:"router_radio,omitempty"`
	// ThisComputer is the name of the interface when the device is this computer's own Wi-Fi card (host.go).
	ThisComputer string `json:"this_computer,omitempty"`
	// OnYourWiFi: it exchanged frames with one of your confirmed (not guest) access points; OwnCycles is in how many cycles.
	OnYourWiFi bool `json:"on_your_wifi,omitempty"`
	OwnCycles  int  `json:"own_cycles,omitempty"`
	// YourAP: it is one of your own confirmed access points.
	YourAP bool `json:"your_ap,omitempty"`
	// Net is the short id of the access point it uses, NetYours whether that is yours.
	Net      string `json:"net,omitempty"`
	NetYours bool   `json:"net_yours,omitempty"`
	// Box is the short id of the router it is a radio of; BoxSize how many radios that router has; BoxYours whether one of them is yours.
	Box      string `json:"box,omitempty"`
	BoxSize  int    `json:"box_size,omitempty"`
	BoxYours bool   `json:"box_yours,omitempty"`
	// Clients is how many devices use it, for an access point.
	Clients int `json:"clients,omitempty"`
	// Confidence is how sure the classifier is of its class (0 to 100).
	Confidence int `json:"confidence"`
	// Pending is the class the classifier now thinks it is and is waiting one more cycle to be sure of (hysteresis); Confidence is then its
	// confidence in the class still held, which can be low.
	Pending Class `json:"pending,omitempty"`
	// Family is how many other private-address devices share its request signature: the same kind of device, never "the same device".
	Family int `json:"family,omitempty"`
	// Motion is still, moving or "" (not enough cycles), from how its signal moved over the last cycles.
	Motion string `json:"motion,omitempty"`
	// FirstLook is a guess from the first readings, before there is enough evidence to sort it (after nodeherder's provisional classes):
	// "mobile" for a private address that is not an access point (someone's phone, watch or earbuds), "neighbour" for a device never heard
	// strong. Shown as a first look, never as a class; empty once the device is sorted.
	FirstLook string `json:"first_look,omitempty"`
}

// The first-look guesses.
const (
	FirstLookMobile    = "mobile"
	FirstLookNeighbour = "neighbour"
)

// firstLook guesses from the first readings, for a device the classifier has not sorted yet.
func firstLook(r FeatureRow, weakDBm int) string {
	if r.Class != "" && r.Class != string(ClassUnknown) {
		return ""
	}
	switch {
	case r.HasMax && r.SigMax <= weakDBm:
		return FirstLookNeighbour
	case r.Random && r.WifiGen == 0:
		return FirstLookMobile
	}
	return ""
}

// The motions.
const (
	MotionStill  = "still"
	MotionMoving = "moving"
	motionMin    = 4
	stillDB      = 2.0
	movingDB     = 4.5
)

// motionUnlessHost is motionOf, except for this computer's own card (self names its interface): it is fixed to the sensor, so a change in its
// signal is the radio's wobble, never a move (as fixedToSensor in live.go).
func motionUnlessHost(trail []float64, self string) string {
	if self != "" {
		return ""
	}
	return motionOf(trail)
}

// motionOf reads still or moving from a trail of per-cycle signals: a still device's signal barely moves, a carried one's swings.
func motionOf(trail []float64) string {
	if len(trail) < motionMin {
		return ""
	}
	sd := spread(trail)
	switch {
	case sd <= stillDB:
		return MotionStill
	case sd >= movingDB:
		return MotionMoving
	}
	return ""
}

func spread(v []float64) float64 {
	m := meanOf(v)
	s := 0.0
	for _, x := range v {
		s += (x - m) * (x - m)
	}
	return math.Sqrt(s / float64(len(v)))
}

// identCtx is what every device's identity is worked out against: the owner's kind answers by maker, the inventory, the boxes, the clients of
// each access point and the request-signature families.
type identCtx struct {
	weakDBm int
	votes   map[string]map[DeviceKind]int
	lan     map[string]LANInfo
	boxSize map[string]int
	boxMine map[string]bool
	clients map[string]int
	apMine  map[string]bool
	family  map[uint64]int
}

func newIdentCtx(rows []FeatureRow, lan map[string]LANInfo) identCtx {
	c := identCtx{weakDBm: DefaultRules().WeakDBm, votes: LearnedKinds(rows), lan: lan, boxSize: map[string]int{}, boxMine: map[string]bool{}, clients: map[string]int{}, apMine: map[string]bool{}, family: map[uint64]int{}}
	for _, r := range rows {
		if len(r.Box) > 0 {
			c.boxSize[string(r.Box)]++
			c.boxMine[string(r.Box)] = c.boxMine[string(r.Box)] || r.OwnAP
		}
		if len(r.Net) > 0 {
			c.clients[string(r.Net)]++
		}
		if r.OwnAP {
			c.apMine[string(r.Tag)] = true
		}
		if r.Random && r.Sig != 0 {
			c.family[r.Sig]++
		}
	}
	return c
}

// of is one device's identity.
func (c identCtx) of(r FeatureRow, trail []float64) DeviceIdent {
	in := kindInputOf(r, c.votes)
	l, onLAN := c.lan[string(r.Tag)]
	if onLAN && !l.Router {
		in.Name = l.Name
	}
	k := GuessKind(in)
	if onLAN && l.Self != "" && k.Source != KindFromOwner {
		k = KindGuess{DeviceComputer, 100, KindFromThisComputer} // only your own word for its kind outranks knowing it is this computer
	}
	id := DeviceIdent{Kind: k.Kind, KindConf: k.Confidence, KindSource: k.Source, Model: r.Model, OnYourWiFi: r.JoinedOwn && r.OwnCycles > 0, OwnCycles: r.OwnCycles,
		YourAP: r.OwnAP, Confidence: r.Conf, Pending: Class(r.Pending), Clients: c.clients[string(r.Tag)], Motion: motionUnlessHost(trail, l.Self)}
	id.Brand, id.BrandHint = brandOf(r)
	id.FirstLook = firstLook(r, c.weakDBm)
	if onLAN {
		id.LANName, id.RouterRadio, id.ThisComputer = l.Name, l.Router, l.Self
	}
	if len(r.Net) > 0 {
		id.Net, id.NetYours = shortID(r.Net), c.apMine[string(r.Net)]
	}
	if len(r.Box) > 0 {
		id.Box, id.BoxSize, id.BoxYours = shortID(r.Box), c.boxSize[string(r.Box)], c.boxMine[string(r.Box)]
	}
	if r.Random && r.Sig != 0 {
		id.Family = c.family[r.Sig] - 1
	}
	return id
}

// brandOf is who made a device, from the strongest source, and whether it is only a hint.
func brandOf(r FeatureRow) (string, bool) {
	switch {
	case r.WPSMaker != "":
		return r.WPSMaker, false
	case r.Maker != "":
		return r.Maker, false
	case r.Hint != "":
		return r.Hint, true
	}
	return "", false
}

// LikelyYours says whether the evidence points to the owner's device without the owner having said so: on the owner's Wi-Fi, in the network
// inventory, or one of the owner's own access points. It is a suggestion the page asks the owner to confirm, never a decision.
func (d DeviceIdent) LikelyYours() bool {
	return d.OnYourWiFi || (d.LANName != "" && !d.RouterRadio) || d.YourAP || d.BoxYours || d.RouterRadio || d.ThisComputer != ""
}
