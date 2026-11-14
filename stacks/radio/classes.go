package radio

import (
	"context"
	"fmt"
	"math"
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
)

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
}

// DefaultRules is version 1: provisional values chosen to be cautious (Unknown until there is evidence), not measured ones.
func DefaultRules() Rules {
	return Rules{
		Version: 1, Provisional: true, WindowDays: 7,
		MinCyclesListened: 12, MinCyclesHeard: 3,
		AlwaysShare: 0.8, SteadyStd: 5,
		StrongDBm: -60, WeakDBm: -75,
		BriefDays: 1, BriefShare: 0.15,
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
}

// FeaturesOf turns a device's rows into features. listened is the cycles the sensor listened through on each day.
func FeaturesOf(r FeatureRow, listened map[int64]int) Features {
	f := Features{Days: r.Days, CyclesHeard: r.Cycles, Random: r.Random, JoinsOwn: r.JoinedOwn}
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
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassInfrastructure, WhyAlwaysSteady, f.Share >= r.AlwaysShare && f.HasSignal && f.SignalStd <= r.SteadyStd && !f.Random
	},
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassTransient, WhyBrief, f.Days <= r.BriefDays && f.Share <= r.BriefShare
	},
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassNeighbour, WhyNeverStrong, f.HasSignal && f.SignalMax <= r.WeakDBm
	},
	func(r Rules, f Features) (Class, Why, bool) {
		return ClassPersonal, WhyStrongOnAndOff, f.HasSignal && f.SignalMax >= r.StrongDBm && f.Share < r.AlwaysShare
	},
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

// CurrentRules reads the rules in force, seeding version 1 the first time.
func (s *Store) CurrentRules(ctx context.Context, at time.Time) (Rules, error) {
	v, err := s.MetaInt(ctx, rulesCurrent)
	if err != nil {
		return Rules{}, err
	}
	if v > 0 {
		var r Rules
		if ok, err := s.GetDoc(ctx, fmt.Sprintf(rulesKey, v), &r); err != nil || ok {
			return r, err
		}
	}
	r := DefaultRules()
	if err := s.PutDoc(ctx, fmt.Sprintf(rulesKey, r.Version), r, at); err != nil {
		return Rules{}, err
	}
	return r, s.SetMetaInt(ctx, rulesCurrent, r.Version)
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
