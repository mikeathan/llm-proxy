package radio

import (
	"context"
	"os"
	"strings"
	"testing"
)

func vec(f Features) vector { return vectorOf(DefaultRules(), f, DeviceUnknown, 0) }

var (
	steadyHere = Features{CyclesListened: 200, CyclesHeard: 190, Share: .95, Days: 7, HasSignal: true, SignalMean: -62, SignalStd: 3, SignalMax: -55}
	passerBy   = Features{CyclesListened: 200, CyclesHeard: 6, Share: .03, Days: 1, HasSignal: true, SignalMean: -74, SignalStd: 3, SignalMax: -68, Random: true}
	farAway    = Features{CyclesListened: 200, CyclesHeard: 120, Share: .6, Days: 6, HasSignal: true, SignalMean: -84, SignalStd: 3, SignalMax: -78}
	myPhone    = Features{CyclesListened: 200, CyclesHeard: 80, Share: .4, Days: 6, HasSignal: true, SignalMean: -55, SignalStd: 8, SignalMax: -40, Random: true}
)

func TestWithNoAnswersTheModelAgreesWithTheRulesOnClearCases(t *testing.T) {
	m := Train(DefaultRules(), nil, 0)
	for name, c := range map[string]struct {
		f    Features
		want Class
	}{"steady": {steadyHere, ClassInfrastructure}, "passer-by": {passerBy, ClassTransient}, "far": {farAway, ClassNeighbour}, "phone": {myPhone, ClassPersonal}} {
		if got, p := top(m.Posterior(vec(c.f))); got != c.want || p < confidenceFloor {
			t.Errorf("%s: %s at %.2f, want %s", name, got, p, c.want)
		}
	}
}

// The walls of this house are thick: the owner's always-on devices are heard weakly, where the rules expect a neighbour's. After the owner says so
// for a few of them, a new device like them is sorted as the house's own.
func TestTheOwnersAnswersMoveTheDecisionAgainstTheRulesThreshold(t *testing.T) {
	r := DefaultRules()
	thickWalls := farAway
	thickWalls.Share, thickWalls.SignalMean, thickWalls.SignalMax, thickWalls.Days = .95, -80, -76, 7
	if got, _ := top(Train(r, nil, 0).Posterior(vec(thickWalls))); got == ClassInfrastructure {
		t.Fatal("the starting point should not already say infrastructure, or this test proves nothing")
	}
	var answers []example
	for i := range 4 {
		f := thickWalls
		f.SignalMean -= float64(i)
		answers = append(answers, example{class: ClassInfrastructure, v: vec(f), weight: ownerWeight, owner: true})
	}
	m := Train(r, answers, 0)
	if got, p := top(m.Posterior(vec(thickWalls))); got != ClassInfrastructure {
		t.Errorf("after four answers the model says %s (%.2f), want infrastructure", got, p)
	}
	if m.Owner != 4 || m.Checked != 4 || m.Agree < 3 {
		t.Errorf("it trained on four answers and gives most back when one is held out: %+v", m)
	}
}

func TestADeviceOnYourWiFiIsNeverANeighboursOrAPasserBy(t *testing.T) {
	f := farAway
	f.JoinsOwn = true
	got, _ := top(Train(DefaultRules(), nil, 0).Posterior(vec(f)))
	if got == ClassNeighbour || got == ClassTransient {
		t.Errorf("on your Wi-Fi: %s", got)
	}
}

// A device whose numbers sit near a boundary must not change class every cycle: a change needs to hold for two cycles in a row.
func TestAClassChangeMustHoldForTwoCyclesBeforeItIsMade(t *testing.T) {
	r := DefaultRules()
	m := Train(r, nil, 0)
	row := FeatureRow{Class: string(ClassInfrastructure), ClassVer: r.Version}
	d := decide(r, m, row, myPhone, vec(myPhone)) // now looks personal
	if d.class != ClassInfrastructure || d.pending != string(ClassPersonal) || d.pendingN != 1 || d.why != WhyHolding {
		t.Fatalf("first cycle holds: %+v", d)
	}
	row.Pending, row.PendingN = d.pending, d.pendingN
	back := decide(r, m, row, steadyHere, vec(steadyHere)) // and back again: the pending change is dropped
	if back.class != ClassInfrastructure || back.pending != "" {
		t.Fatalf("back to its class: %+v", back)
	}
	row.Pending, row.PendingN = d.pending, d.pendingN
	second := decide(r, m, row, myPhone, vec(myPhone))
	if second.class != ClassPersonal {
		t.Errorf("two cycles in a row: %+v", second)
	}
}

func TestAnUnsureAnswerIsUnknownNotAGuess(t *testing.T) {
	r := DefaultRules()
	m := Train(r, nil, 0)
	mid := Features{CyclesListened: 200, CyclesHeard: 100, Share: .5, Days: 4, HasSignal: true, SignalMean: -70, SignalStd: 5, SignalMax: -62}
	_, p := top(m.Posterior(vec(mid)))
	d := decide(r, m, FeatureRow{}, mid, vec(mid))
	if p < confidenceFloor && d.class != ClassUnknown {
		t.Errorf("at %.2f the answer should be unknown: %+v", p, d)
	}
	if d.conf < 0 || d.conf > 100 {
		t.Errorf("confidence out of range: %d", d.conf)
	}
}

func TestExplainNamesTheNumbersBehindADecision(t *testing.T) {
	m := Train(DefaultRules(), nil, 0)
	got := m.Explain(vec(farAway), ClassNeighbour)
	if len(got) == 0 || len(got) > maxFactors {
		t.Fatalf("%+v", got)
	}
	found := false
	for _, f := range got {
		found = found || f.Feature == "signal_mean" || f.Feature == "signal_max"
	}
	if !found {
		t.Errorf("a far device is a neighbour's because of its signal: %+v", got)
	}
}

func TestSelfTrainingCannotOutweighTheCap(t *testing.T) {
	m := newModel(DefaultRules())
	for range 1000 {
		m.add(ClassTransient, vec(passerBy), selfWeight, true)
	}
	if st := m.Stats[ClassTransient]; st.Self > selfCap+1e-9 || st.W > priorWeight+selfCap+1e-9 {
		t.Errorf("self-training ran away: %+v", st)
	}
}

func TestTheLearnedModelIsKeptWithNoDeviceInIt(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	for range 14 {
		cycle(t, r, data(neighbour), data(macPhone))
	}
	m, ok, err := s.LearnedModel(context.Background())
	if err != nil || !ok || m.Version != DefaultRules().Version || len(m.Stats) != len(learnClasses) {
		t.Fatalf("%+v %v %v", m, ok, err)
	}
}

func TestKindsComeFromTheStrongestEvidence(t *testing.T) {
	learned := map[string]map[DeviceKind]int{"espressif inc.": {DeviceCamera: 2}}
	for name, c := range map[string]struct {
		in   KindInput
		want DeviceKind
		src  KindSource
	}{
		"owner":          {KindInput{OwnerKind: DeviceTV, AccessPoint: true}, DeviceTV, KindFromOwner},
		"wps type":       {KindInput{Category: wpsDisplay, AccessPoint: true}, DeviceTV, KindFromWPSType},
		"model":          {KindInput{Model: "Fire TV Stick", Maker: "Amazon"}, DeviceTV, KindFromModel},
		"learned":        {KindInput{Maker: "Espressif Inc.", Learned: learned}, DeviceCamera, KindFromLearned},
		"beacons":        {KindInput{AccessPoint: true, Maker: "Apple"}, DeviceRouter, KindFromBeacons},
		"maker":          {KindInput{Maker: "Arcadyan"}, DeviceRouter, KindFromMaker},
		"random no mkr":  {KindInput{Maker: "Arcadyan", Random: true}, DevicePhone, KindFromPrivate},
		"apple hint":     {KindInput{Hint: "Apple", Random: true}, DevicePhone, KindFromHint},
		"nothing at all": {KindInput{}, DeviceUnknown, KindFromNothing},
	} {
		g := GuessKind(c.in)
		if g.Kind != c.want || g.Source != c.src {
			t.Errorf("%s: %+v", name, g)
		}
	}
}

func TestLearnedKindsCountOnlyTheOwnersAnswersAndNeverAPrivateAddress(t *testing.T) {
	rows := []FeatureRow{
		{Maker: "Espressif Inc.", Kind: "camera", KindBy: kindByOwner},
		{Maker: "Espressif Inc.", Kind: "smart_home", KindBy: ""},
		{Maker: "Apple", Kind: "phone", KindBy: kindByOwner, Random: true},
	}
	got := LearnedKinds(rows)
	if got["espressif inc."][DeviceCamera] != 1 || got["espressif inc."][DeviceSmartHome] != 0 || len(got) != 1 {
		t.Errorf("%+v", got)
	}
}

// Every kind, kind source and classifier factor the server can send has its words on the page, so none shows as a raw id.
func TestEveryKindSourceAndFactorHasItsWordsOnThePage(t *testing.T) {
	page, err := os.ReadFile("../../frontend/src/constants/radioIdentity.ts")
	if err != nil {
		t.Skip("the frontend is not beside the stack")
	}
	words := string(page)
	for _, s := range KindSources() {
		if !strings.Contains(words, "  "+string(s)+": ") {
			t.Errorf("kind source %q has no words (RADIO_KIND_SOURCES)", s)
		}
	}
	for _, k := range DeviceKinds {
		if !strings.Contains(words, "  "+string(k)+": ") && !strings.Contains(words, " "+string(k)+": ") {
			t.Errorf("kind %q has no words (RADIO_KIND_WORDS)", k)
		}
	}
	for _, f := range featureNames {
		if !strings.Contains(words, "  "+f+": ") {
			t.Errorf("factor %q has no words (RADIO_FACTOR_WORDS)", f)
		}
	}
}

func TestASortedDeviceIsNotDroppedToUnknownByOneUnsureCycle(t *testing.T) {
	r := DefaultRules()
	m := newModel(r)
	for _, c := range learnClasses { // every class alike: the model cannot tell them apart, so it is unsure of everything
		st := *m.Stats[ClassInfrastructure]
		m.Stats[c] = &st
	}
	mid := Features{CyclesListened: 200, CyclesHeard: 100, Share: .5, Days: 4, HasSignal: true, SignalMean: -70, SignalStd: 5, SignalMax: -62}
	if _, p := top(m.Posterior(vec(mid))); p >= confidenceFloor {
		t.Fatalf("the fixture must be unsure: %.2f", p)
	}
	d := decide(r, m, FeatureRow{Class: "personal", ClassVer: r.Version, Pending: "x", PendingN: 3}, mid, vec(mid))
	if d.class != ClassPersonal || d.pending != "" || d.pendingN != 0 {
		t.Errorf("kept, with nothing pending: %+v", d)
	}
}
