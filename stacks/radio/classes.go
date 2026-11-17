package radio

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Classes (plan §4). A class is a rule over features, and the rules are data: a versioned document in the database, so a change is a new
// version, logged, and nothing is reclassified silently. The numbers in version 1 are PROVISIONAL starting values, not measurements: the
// capture the owner made is not in the repository, so no threshold has been fitted to vertex's air yet. The rules carry that word (Provisional)
// and the page says it, until a later version is fitted to measured data and the owner's labels.

// Class is what kind of device something is, as far as the evidence goes.
type Class string

// The classes the owner's table lists, and Unknown for too little evidence.
const (
	ClassInfrastructure Class = "infrastructure" // present all the time, steady signal: TV, bulbs, plugs
	ClassPersonal       Class = "personal"       // comes and goes, strong when here: phones, watches
	ClassNeighbour      Class = "neighbour"      // consistently weak, never strong: outside the owner's walls
	ClassTransient      Class = "transient"      // brief appearances: passers-by, cars
	ClassUnknown        Class = "unknown"        // too little evidence, or no rule fits
)

// Classes lists them in the order the page shows them.
var Classes = []Class{ClassInfrastructure, ClassPersonal, ClassNeighbour, ClassTransient, ClassUnknown}

// Why is the fixed reason word a class carries. The page words it; a device never writes one.
type Why string

// The reasons.
const (
	WhyNeedsMore       Why = "needs_more_listening"
	WhyJoinsAlways     Why = "joins_your_network_always_here"
	WhyJoinsSometimes  Why = "joins_your_network_comes_and_goes"
	WhyAlwaysSteady    Why = "always_here_steady_signal"
	WhyBrief           Why = "brief_appearance"
	WhyNeverStrong     Why = "never_strong"
	WhyStrongOnAndOff  Why = "strong_and_comes_and_goes"
	WhyNoRuleFits      Why = "no_rule_fits"
	WhyOwnerSaid       Why = "owner_said"
	WhyFirstClassified Why = "first_look"
	WhyNotHeardLately  Why = "not_heard_lately"
	// Version 2 (2026-11-16, from what vertex's air showed): an access point is a device whose beacon or probe response announced a Wi-Fi generation.
	WhyAccessPointAlways    Why = "access_point_always_here"
	WhyAccessPointSometimes Why = "access_point_comes_and_goes"
	WhyAlwaysVaries         Why = "always_here_signal_varies"
	WhySmartHomeMaker       Why = "maker_is_smart_home"
)

// Whys lists every reason the rules can give, so a test can hold the page's table of plain words to it.
func Whys() []Why {
	return []Why{WhyNeedsMore, WhyJoinsAlways, WhyJoinsSometimes, WhyAlwaysSteady, WhyBrief, WhyNeverStrong, WhyStrongOnAndOff, WhyNoRuleFits,
		WhyOwnerSaid, WhyFirstClassified, WhyNotHeardLately, WhyAccessPointAlways, WhyAccessPointSometimes, WhyAlwaysVaries, WhySmartHomeMaker}
}

// Rules is one version of the classifier.
type Rules struct {
	Version     int  `json:"version"`
	Provisional bool `json:"provisional"`
	// WindowDays is how many days back the features look (at most the retention of other people's rows).
	WindowDays int `json:"window_days"`
	// MinCyclesListened is how long the sensor must have listened (in cycles) in the window before anything but Unknown is said, and
	// MinCyclesHeard is how often a device must have been heard.
	MinCyclesListened int `json:"min_cycles_listened"`
	MinCyclesHeard    int `json:"min_cycles_heard"`
	// AlwaysShare is the share of cycles a device is heard in to count as always here; SteadyStd is the most its signal may spread (dB).
	AlwaysShare float64 `json:"always_share"`
	SteadyStd   float64 `json:"steady_std_db"`
	// StrongDBm and WeakDBm: a device whose best signal reached StrongDBm is near; one that never got above WeakDBm is outside the walls.
	StrongDBm int `json:"strong_dbm"`
	WeakDBm   int `json:"weak_dbm"`
	// BriefDays and BriefShare: a device seen on at most BriefDays days and in at most BriefShare of the cycles is a passer-by.
	BriefDays  int     `json:"brief_days"`
	BriefShare float64 `json:"brief_share"`
	// SmartHomeMakers are parts of a maker's name (lower case) whose devices are smart-home modules: always-on things that are heard part of the time.
	// A small fixed list, provisional like the rest; a device on a random address is never sorted by its maker.
	SmartHomeMakers []string `json:"smart_home_makers"`
}

// DefaultRules is version 2 (version 1 plus access points, always-here devices whose signal is not steady, and the smart-home makers): provisional
// values chosen to be cautious (Unknown until there is evidence), not measured ones.
func DefaultRules() Rules {
	return Rules{
		Version: 2, Provisional: true, WindowDays: 7,
		MinCyclesListened: 12, MinCyclesHeard: 3,
		AlwaysShare: 0.8, SteadyStd: 5,
		StrongDBm: -60, WeakDBm: -75,
		BriefDays: 1, BriefShare: 0.15,
		SmartHomeMakers: []string{"espressif", "tuya", "shelly"}, // each matches the embedded registry (checked: 420 Espressif or Tuya blocks, 1 Shelly)
	}
}

// Features is what a device has done over the rules' window, as numbers.
type Features struct {
	Days, CyclesHeard, CyclesListened int
	Share                             float64
	SignalMean, SignalStd             float64
	SignalMax                         int
	HasSignal                         bool
	Random, JoinsOwn                  bool
	// AccessPoint is set when a beacon or probe response from the device announced a Wi-Fi generation (a phone's probe request does not; a phone hotspot does); Maker is who its address block was assigned to.
	AccessPoint bool
	Maker       string
}

// FeaturesOf turns a device's rows into features. listened is the cycles the sensor listened through on each day.
func FeaturesOf(r FeatureRow, listened map[int64]int) Features {
	f := Features{Days: r.Days, CyclesHeard: r.Cycles, Random: r.Random, JoinsOwn: r.JoinedOwn, AccessPoint: r.WifiGen > 0, Maker: r.Maker}
	for day, n := range listened {
		if day >= r.FirstDay {
			f.CyclesListened += n
		}
	}
	if f.CyclesListened > 0 {
		f.Share = math.Min(1, float64(f.CyclesHeard)/float64(f.CyclesListened))
	}
	if r.SigN > 0 && r.HasMax {
		mean := float64(r.SigSum) / float64(r.SigN)
		f.SignalMean, f.SignalMax, f.HasSignal = mean, r.SigMax, true
		f.SignalStd = math.Sqrt(math.Max(0, float64(r.SigSq)/float64(r.SigN)-mean*mean))
	}
	return f
}

// rule is one step of the classifier: it answers when it applies and says nothing otherwise.
type rule func(r Rules, f Features) (Class, Why, bool)

// ruleOrder is the strength of the evidence, strongest first: too little evidence stops everything, then joining the owner's own network.
var ruleOrder = []rule{
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassUnknown, WhyNeedsMore, f.CyclesListened < r.MinCyclesListened || f.CyclesHeard < r.MinCyclesHeard
	},
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassInfrastructure, WhyJoinsAlways, f.JoinsOwn && f.Share >= r.AlwaysShare
	},
	func(_ Rules, f Features) (Class, Why, bool) { return ClassPersonal, WhyJoinsSometimes, f.JoinsOwn },
	// An access point that was never strong is a neighbour's; one that is always here is the house's own (its signal wobbles, so steadiness is not asked).
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassNeighbour, WhyNeverStrong, f.AccessPoint && f.HasSignal && f.SignalMax <= r.WeakDBm
	},
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassInfrastructure, WhyAccessPointAlways, f.AccessPoint && f.Share >= r.AlwaysShare
	},
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassInfrastructure, WhyAlwaysSteady, f.Share >= r.AlwaysShare && f.HasSignal && f.SignalStd <= r.SteadyStd && !f.Random
	},
	// Heard in nearly every cycle, signal steady or not: always on. (Version 1 had no rule here and left seven such devices unknown.)
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassInfrastructure, WhyAlwaysVaries, f.Share >= r.AlwaysShare && !(f.HasSignal && f.SignalMax <= r.WeakDBm)
	},
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassTransient, WhyBrief, f.Days <= r.BriefDays && f.Share <= r.BriefShare
	},
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassNeighbour, WhyNeverStrong, f.HasSignal && f.SignalMax <= r.WeakDBm
	},
	// A smart-home module is heard part of the time and sits in one place; a random address says nothing about its maker, so it is left to the rules below.
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassInfrastructure, WhySmartHomeMaker, !f.Random && r.smartHome(f.Maker)
	},
	// A router that comes and goes is somebody's access point, never a personal device.
	func(_ Rules, f Features) (Class, Why, bool) {
		return ClassNeighbour, WhyAccessPointSometimes, f.AccessPoint
	},
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassPersonal, WhyStrongOnAndOff, f.HasSignal && f.SignalMax >= r.StrongDBm && f.Share < r.AlwaysShare
	},
}

// smartHome says whether a maker's name is on the rules' list of smart-home module makers.
func (r Rules) smartHome(maker string) bool {
	name := strings.ToLower(maker)
	return name != "" && slices.ContainsFunc(r.SmartHomeMakers, func(part string) bool { return part != "" && strings.Contains(name, part) })
}

// Classify applies the rules in order and says why; when none applies the answer is Unknown, which is not a guess.
func (r Rules) Classify(f Features) (Class, Why) {
	for _, rl := range ruleOrder {
		if c, why, ok := rl(r, f); ok {
			return c, why
		}
	}
	return ClassUnknown, WhyNoRuleFits
}

const (
	rulesKey     = "classifier_v%d"
	rulesCurrent = "classifier_current"
)

// CurrentRules reads the rules in force. A store that holds an older version than this code carries is moved to the code's: the new version is
// published (its document is kept beside the old one) and becomes current, so the next ClassifyAll looks at every device again and logs each
// class that changed with the new version and its reason. A newer version in the store (a fitted one) is left alone.
func (s *Store) CurrentRules(ctx context.Context, at time.Time) (Rules, error) {
	v, err := s.MetaInt(ctx, rulesCurrent)
	if err != nil {
		return Rules{}, err
	}
	latest := DefaultRules()
	if v >= latest.Version {
		var r Rules
		if ok, err := s.GetDoc(ctx, fmt.Sprintf(rulesKey, v), &r); err != nil || ok {
			return r, err
		}
	}
	if err := s.PutDoc(ctx, fmt.Sprintf(rulesKey, latest.Version), latest, at); err != nil {
		return Rules{}, err
	}
	return latest, s.SetMetaInt(ctx, rulesCurrent, latest.Version)
}

// ClassifyAll runs the rules over every device with rows in the window and records each class that changed, with the reason. A device whose class
// the owner set is left alone. It returns how many devices changed class.
func (s *Store) ClassifyAll(ctx context.Context, now time.Time) (int, error) {
	rules, err := s.CurrentRules(ctx, now)
	if err != nil {
		return 0, err
	}
	rows, listened, err := s.FeatureRows(ctx, dayNumber(now)-int64(rules.WindowDays)+1)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, r := range rows {
		if r.Class != "" && r.ClassVer == 0 {
			continue // the owner's word
		}
		if r.Class != "" && r.Cycles == 0 {
			continue // sorted once and not heard in the window: no new evidence, so no change (a silent week is not "not enough evidence")
		}
		cls, why := rules.Classify(FeaturesOf(r, listened))
		if string(cls) == r.Class && r.ClassVer == rules.Version {
			continue
		}
		if err := s.SetClass(ctx, r.Tag, r.Owner, r.Class, string(cls), rules.Version, string(why), now); err != nil {
			return changed, err
		}
		if string(cls) != r.Class {
			changed++
		}
	}
	return changed, nil
}
