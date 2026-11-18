package radio

import (
	"strings"
)

// What a device is (ADR-0058): a router, a phone, a TV. Separate from its class (how it behaves around the house): a router can be the house's own
// or a neighbour's. The kind comes from the strongest evidence there is, and the answer says which evidence and how sure it is:
//
//  1. the owner's word;
//  2. the WPS device type the device announced;
//  3. its model name;
//  4. what the owner said about other devices of the same maker (learned: label spreading, ADR-0058);
//  5. sending beacons (an access point);
//  6. the maker of its address block, when that maker makes one kind of thing;
//  7. a vendor element's company;
//  8. a private (random) address, which phones, tablets and laptops use.
//
// Every answer below the owner's is a guess and is worded as one.

// DeviceKind is what a device is.
type DeviceKind string

// The kinds.
const (
	DeviceRouter    DeviceKind = "router"
	DevicePhone     DeviceKind = "phone"
	DeviceComputer  DeviceKind = "computer"
	DeviceTV        DeviceKind = "tv"
	DeviceSpeaker   DeviceKind = "speaker"
	DeviceSmartHome DeviceKind = "smart_home"
	DeviceCamera    DeviceKind = "camera"
	DeviceWearable  DeviceKind = "wearable"
	DevicePrinter   DeviceKind = "printer"
	DeviceConsole   DeviceKind = "console"
	DeviceUnknown   DeviceKind = "unknown"
)

// DeviceKinds lists every kind, in the order the page offers them.
var DeviceKinds = []DeviceKind{DeviceRouter, DevicePhone, DeviceComputer, DeviceTV, DeviceSpeaker, DeviceSmartHome, DeviceCamera, DeviceWearable, DevicePrinter, DeviceConsole, DeviceUnknown}

// KindSource is the evidence a kind came from; the page words it.
type KindSource string

// The sources, strongest first.
const (
	KindFromOwner   KindSource = "owner_said"
	KindFromWPSType KindSource = "announced_type"
	KindFromModel   KindSource = "model_name"
	KindFromLearned KindSource = "learned_from_you"
	KindFromBeacons KindSource = "sends_beacons"
	KindFromMaker   KindSource = "maker"
	KindFromHint    KindSource = "vendor_element"
	KindFromPrivate KindSource = "private_address"
	KindFromNothing KindSource = "no_evidence"
	kindByOwner                = "owner"
	learnedMinVotes            = 1
	learnedMinShare            = 0.66
)

// KindSources lists every source, so a test can hold the page's words to it.
func KindSources() []KindSource {
	return []KindSource{KindFromOwner, KindFromWPSType, KindFromModel, KindFromLearned, KindFromBeacons, KindFromMaker, KindFromHint, KindFromPrivate, KindFromNothing}
}

// KindInput is what is known about a device that bears on its kind.
type KindInput struct {
	OwnerKind   DeviceKind
	AccessPoint bool
	Random      bool
	Maker       string // from the address block
	WPSMaker    string // from the WPS element
	Model       string
	Category    int
	Hint        string
	// Learned is what the owner said about other devices of each maker: maker (lower case) to kind to how many.
	Learned map[string]map[DeviceKind]int
}

// KindGuess is the answer: the kind, how sure (0 to 100) and from what.
type KindGuess struct {
	Kind       DeviceKind `json:"kind"`
	Confidence int        `json:"confidence"`
	Source     KindSource `json:"source"`
}

// kindStep is one rung of the evidence ladder.
type kindStep func(in KindInput) (KindGuess, bool)

var kindLadder = []kindStep{
	func(in KindInput) (KindGuess, bool) {
		return KindGuess{in.OwnerKind, 100, KindFromOwner}, in.OwnerKind != "" && in.OwnerKind != DeviceUnknown
	},
	func(in KindInput) (KindGuess, bool) {
		k, ok := wpsKinds[in.Category]
		return KindGuess{k, 85, KindFromWPSType}, ok
	},
	func(in KindInput) (KindGuess, bool) {
		k, ok := lexicon(modelWords, in.Model)
		return KindGuess{k, 80, KindFromModel}, ok
	},
	learnedKind,
	func(in KindInput) (KindGuess, bool) {
		return KindGuess{DeviceRouter, 90, KindFromBeacons}, in.AccessPoint
	},
	func(in KindInput) (KindGuess, bool) {
		if in.Random {
			return KindGuess{}, false // a private address names no maker
		}
		k, ok := lexicon(makerWords, in.Maker+" "+in.WPSMaker)
		return KindGuess{k, 60, KindFromMaker}, ok
	},
	func(in KindInput) (KindGuess, bool) {
		k, ok := hintKinds[in.Hint]
		return KindGuess{k, 45, KindFromHint}, ok
	},
	func(in KindInput) (KindGuess, bool) { return KindGuess{DevicePhone, 35, KindFromPrivate}, in.Random },
}

// GuessKind climbs the ladder and gives the first answer.
func GuessKind(in KindInput) KindGuess {
	for _, step := range kindLadder {
		if g, ok := step(in); ok {
			return g
		}
	}
	return KindGuess{DeviceUnknown, 0, KindFromNothing}
}

// learnedKind is the kind the owner gave most often to other devices of this maker, when the owner's answers agree enough. It grows surer with
// each answer: one answer gives 65, each more adds 5, up to 85.
func learnedKind(in KindInput) (KindGuess, bool) {
	maker := makerKey(in)
	votes := in.Learned[maker]
	if maker == "" || len(votes) == 0 {
		return KindGuess{}, false
	}
	var best DeviceKind
	n, total := 0, 0
	for k, c := range votes {
		total += c
		if c > n || (c == n && k < best) {
			best, n = k, c
		}
	}
	if n < learnedMinVotes || float64(n) < learnedMinShare*float64(total) {
		return KindGuess{}, false
	}
	return KindGuess{best, min(85, 60+5*n), KindFromLearned}, true
}

// makerKey is the maker a learned answer is filed under: the address block's maker, else the one the WPS element named; never for a private
// address, whose maker is unknown.
func makerKey(in KindInput) string {
	if in.Random {
		return ""
	}
	if in.Maker != "" {
		return strings.ToLower(in.Maker)
	}
	return strings.ToLower(in.WPSMaker)
}

// lexicon finds the first word of a list in the text (lower case).
func lexicon(words []kindWord, text string) (DeviceKind, bool) {
	t := strings.ToLower(text)
	if strings.TrimSpace(t) == "" {
		return "", false
	}
	for _, w := range words {
		if strings.Contains(t, w.part) {
			return w.kind, true
		}
	}
	return "", false
}

type kindWord struct {
	part string
	kind DeviceKind
}

// wpsKinds maps the WPS primary device type categories to kinds.
var wpsKinds = map[int]DeviceKind{
	wpsComputer: DeviceComputer, wpsPrinter: DevicePrinter, wpsCamera: DeviceCamera, wpsNetwork: DeviceRouter, wpsDisplay: DeviceTV,
	wpsMedia: DeviceTV, wpsGaming: DeviceConsole, wpsPhone: DevicePhone, wpsAudio: DeviceSpeaker,
}

// hintKinds: a vendor element names a company; only Apple's says much about the device (iPhones send it; so do Macs and iPads).
var hintKinds = map[string]DeviceKind{"Apple": DevicePhone}

// modelWords are parts of a model name, most specific first.
var modelWords = []kindWord{
	{"chromecast", DeviceTV}, {"fire tv", DeviceTV}, {"firetv", DeviceTV}, {"roku", DeviceTV}, {"apple tv", DeviceTV}, {"bravia", DeviceTV}, {"smart tv", DeviceTV},
	{"sky q", DeviceTV}, {"now tv", DeviceTV}, {"stick", DeviceTV}, {" tv", DeviceTV}, {"echo", DeviceSpeaker}, {"sonos", DeviceSpeaker}, {"homepod", DeviceSpeaker},
	{"nest", DeviceSmartHome}, {"hue", DeviceSmartHome}, {"plug", DeviceSmartHome}, {"bulb", DeviceSmartHome}, {"thermostat", DeviceSmartHome},
	{"doorbell", DeviceCamera}, {"cam", DeviceCamera}, {"printer", DevicePrinter}, {"laserjet", DevicePrinter}, {"deskjet", DevicePrinter}, {"officejet", DevicePrinter},
	{"envy", DevicePrinter}, {"pixma", DevicePrinter}, {"xbox", DeviceConsole}, {"playstation", DeviceConsole}, {"switch", DeviceConsole},
	{"iphone", DevicePhone}, {"galaxy", DevicePhone}, {"pixel", DevicePhone}, {"ipad", DevicePhone}, {"watch", DeviceWearable},
	{"macbook", DeviceComputer}, {"laptop", DeviceComputer}, {"windows", DeviceComputer},
	{"hub", DeviceRouter}, {"router", DeviceRouter}, {"extender", DeviceRouter}, {"repeater", DeviceRouter}, {"mesh", DeviceRouter}, {"booster", DeviceRouter},
	{"archer", DeviceRouter}, {"deco", DeviceRouter}, {"orbi", DeviceRouter}, {"rt-", DeviceRouter}, {"fritz", DeviceRouter}, {"eero", DeviceRouter},
}

// makerWords are makers that make mostly one kind of thing. A maker of several kinds (Apple, Samsung, Sony, Google, Amazon) is not here: its
// address says nothing about which. A Wi-Fi module maker inside other products (Espressif, Tuya) is a smart-home guess.
var makerWords = []kindWord{
	{"arcadyan", DeviceRouter}, {"sagemcom", DeviceRouter}, {"technicolor", DeviceRouter}, {"askey", DeviceRouter}, {"sercomm", DeviceRouter}, {"zte", DeviceRouter},
	{"tp-link", DeviceRouter}, {"netgear", DeviceRouter}, {"asustek", DeviceRouter}, {"linksys", DeviceRouter}, {"ubiquiti", DeviceRouter}, {"avm", DeviceRouter},
	{"zyxel", DeviceRouter}, {"eero", DeviceRouter}, {"plume", DeviceRouter}, {"mikrotik", DeviceRouter}, {"draytek", DeviceRouter}, {"sky uk", DeviceRouter},
	{"british telecommunications", DeviceRouter}, {"vodafone", DeviceRouter}, {"virgin media", DeviceRouter},
	{"espressif", DeviceSmartHome}, {"tuya", DeviceSmartHome}, {"shelly", DeviceSmartHome}, {"signify", DeviceSmartHome}, {"philips lighting", DeviceSmartHome},
	{"lifx", DeviceSmartHome}, {"itead", DeviceSmartHome}, {"belkin", DeviceSmartHome}, {"ecobee", DeviceSmartHome}, {"nest labs", DeviceSmartHome},
	{"tado", DeviceSmartHome}, {"hive", DeviceSmartHome}, {"meross", DeviceSmartHome}, {"wiz connected", DeviceSmartHome},
	{"ring", DeviceCamera}, {"arlo", DeviceCamera}, {"wyze", DeviceCamera}, {"hikvision", DeviceCamera}, {"dahua", DeviceCamera}, {"reolink", DeviceCamera},
	{"eufy", DeviceCamera}, {"blink", DeviceCamera},
	{"roku", DeviceTV}, {"lg electronics", DeviceTV}, {"vizio", DeviceTV}, {"hisense", DeviceTV}, {"tcl", DeviceTV}, {"gaoshengda", DeviceTV}, {"gsd viet nam", DeviceTV},
	{"sonos", DeviceSpeaker}, {"bose", DeviceSpeaker},
	{"hewlett packard", DevicePrinter}, {"hp inc", DevicePrinter}, {"canon", DevicePrinter}, {"seiko epson", DevicePrinter}, {"brother", DevicePrinter},
	{"nintendo", DeviceConsole}, {"sony interactive", DeviceConsole},
	{"garmin", DeviceWearable}, {"fitbit", DeviceWearable},
	{"honor device", DevicePhone}, {"xiaomi", DevicePhone}, {"oneplus", DevicePhone}, {"oppo", DevicePhone}, {"vivo mobile", DevicePhone}, {"motorola", DevicePhone},
	{"intel corporate", DeviceComputer}, {"liteon", DeviceComputer}, {"azurewave", DeviceComputer}, {"hon hai", DeviceComputer}, {"dell", DeviceComputer},
	{"lenovo", DeviceComputer},
}

// validKind says whether a kind is one of the list.
func validKind(k DeviceKind) bool {
	for _, d := range DeviceKinds {
		if d == k {
			return true
		}
	}
	return false
}

// LearnedKinds counts what the owner said about devices, by maker: the label-spreading table. Only the owner's own answers count, and only for
// devices whose address names a maker.
func LearnedKinds(rows []FeatureRow) map[string]map[DeviceKind]int {
	out := map[string]map[DeviceKind]int{}
	for _, r := range rows {
		if r.KindBy != kindByOwner || r.Kind == "" || r.Kind == string(DeviceUnknown) {
			continue
		}
		key := makerKey(KindInput{Random: r.Random, Maker: r.Maker, WPSMaker: r.WPSMaker})
		if key == "" {
			continue
		}
		if out[key] == nil {
			out[key] = map[DeviceKind]int{}
		}
		out[key][DeviceKind(r.Kind)]++
	}
	return out
}

// KindOf is a device's kind from its row. Its own answer is never counted as learned evidence for itself: the owner's word is rung 1.
func KindOf(r FeatureRow, learned map[string]map[DeviceKind]int) KindGuess {
	in := KindInput{AccessPoint: r.WifiGen > 0, Random: r.Random, Maker: r.Maker, WPSMaker: r.WPSMaker, Model: r.Model, Category: r.Category, Hint: r.Hint, Learned: learned}
	if r.KindBy == kindByOwner {
		in.OwnerKind = DeviceKind(r.Kind)
	}
	return GuessKind(in)
}
