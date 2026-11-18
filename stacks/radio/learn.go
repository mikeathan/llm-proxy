package radio

import (
	"math"
	"sort"
)

// The learned classifier (ADR-0058). The hand-written rules (classes.go) say what each class usually looks like; this file turns that into a
// model that the house's own data and the owner's answers move:
//
//   - A naive Bayes model over a few numbers per device (how often it is heard, its signal, how steady, its strongest, how many days, whether it
//     sends beacons, has a private address, uses the owner's Wi-Fi, is a fixed kind of thing).
//   - Its starting point is a small number of made-up examples per class written from the rules (the prior), so with no labels it behaves like the rules.
//   - It is trained again every cycle from the devices in the window: the owner's own answers weigh most, hard evidence (on the owner's Wi-Fi, a
//     never-strong access point) next, and the rules' own answers a little (self-training, capped, so it cannot run away from what it saw).
//     Training afresh each time, with nothing carried over but the result, means nothing drifts: what it believes is always what the window shows.
//   - A device moves class only when the new class is clearly ahead and stays ahead for two cycles in a row (hysteresis), so the class log does not
//     fill with flips. Below a confidence floor the answer is Unknown.
//
// It stores only per-class sums (no tag, no device) in a document, so the page can show what it learned and how well it agrees with the owner.

const (
	priorWeight     = 4.0  // made-up examples per class from the rules
	ownerWeight     = 6.0  // one answer of the owner's
	evidenceWeight  = 2.0  // a hard-evidence case
	selfWeight      = 0.3  // a device the rules placed themselves
	selfCap         = 40.0 // the most weight self-training may add to one class
	minVariance     = 1e-3
	confidenceFloor = 0.55 // below this the answer is Unknown
	switchMargin    = 0.15 // how far ahead a new class must be
	switchCycles    = 2    // for how many classifications in a row
	learnedDoc      = "classifier_learned"
	maxFactors      = 3
	halfShare       = 0.5 // the share of each of two classes the model cannot tell apart
)

// The numbers the model reads, in this order.
const (
	fShare = iota
	fSigMean
	fSigStd
	fSigMax
	fDays
	fNight
	fCross
	numContinuous
)

const (
	bAP = iota
	bRandom
	bOnOwn
	bFixedKind
	numBinary
)

// featureNames are the words a factor is reported with; the page words them.
var featureNames = [numContinuous + numBinary]string{"share", "signal_mean", "signal_spread", "signal_max", "days", "night", "movement", "access_point", "private_address", "uses_your_wifi", "fixed_kind"}

// learnClasses are the classes the model chooses between; Unknown is what it says when it cannot.
var learnClasses = []Class{ClassInfrastructure, ClassPersonal, ClassNeighbour, ClassTransient}

// vector is one device as numbers. hasSig is false when no signal was measured, and the signal numbers are then left out.
type vector struct {
	c      [numContinuous]float64
	b      [numBinary]bool
	hasSig bool
}

// classStats are the weighted sums of one class's examples.
type classStats struct {
	W    float64                `json:"w"`
	SigW float64                `json:"sig_w"`
	Sum  [numContinuous]float64 `json:"sum"`
	Sq   [numContinuous]float64 `json:"sq"`
	B    [numBinary]float64     `json:"b"`
	Self float64                `json:"self"`
}

// Model is the learned classifier: the sums per class, how many owner answers and evidence cases trained it, and how well it agrees with the owner.
type Model struct {
	Version   int                   `json:"version"`
	Stats     map[Class]*classStats `json:"stats"`
	Owner     int                   `json:"owner_labels"`
	Evidence  int                   `json:"evidence_labels"`
	Self      int                   `json:"self_labels"`
	Agree     int                   `json:"agree"`
	Checked   int                   `json:"checked"`
	TrainedAt int64                 `json:"trained_at"`
}

// priorExample is what a class looks like according to the rules: the mean and spread of each number and the chance of each yes-or-no.
type priorExample struct {
	mean, sd [numContinuous]float64
	p        [numBinary]float64
}

// priorsFrom writes the rules as prior examples. Signal levels follow the rules' strong and weak thresholds, so a fitted rules version moves them.
func priorsFrom(r Rules) map[Class]priorExample {
	strong, weak := float64(r.StrongDBm), float64(r.WeakDBm)
	return map[Class]priorExample{
		ClassInfrastructure: {mean: [numContinuous]float64{0.92, (strong + weak) / 2, 4, strong + 5, 0.9, 0.85, 0.02}, sd: [numContinuous]float64{0.1, 10, 3, 10, 0.2, 0.25, 0.05}, p: [numBinary]float64{0.45, 0.1, 0.5, 0.75}},
		ClassPersonal:       {mean: [numContinuous]float64{0.4, strong, 7, strong + 12, 0.6, 0.35, 0.15}, sd: [numContinuous]float64{0.25, 10, 4, 10, 0.3, 0.35, 0.15}, p: [numBinary]float64{0.05, 0.75, 0.5, 0.1}},
		ClassNeighbour:      {mean: [numContinuous]float64{0.6, weak - 7, 4, weak, 0.7, 0.5, 0.03}, sd: [numContinuous]float64{0.3, 6, 3, 6, 0.3, 0.4, 0.06}, p: [numBinary]float64{0.4, 0.3, 0.02, 0.4}},
		ClassTransient:      {mean: [numContinuous]float64{0.05, weak, 4, weak + 7, 0.15, 0.1, 0.05}, sd: [numContinuous]float64{0.06, 10, 3, 10, 0.15, 0.2, 0.1}, p: [numBinary]float64{0.1, 0.7, 0.02, 0.1}},
	}
}

// The signal reference (ADR-0058, after nodeherder's "below the strongest device in the room"): an adapter's gain and where it sits move every
// reading by the same amount, so the model reads signals relative to the owner's own access points, shifted so that their strongest reading
// sits at referenceDBm. With no access point of the owner's heard, nothing is shifted. The shift is capped so one odd reading cannot move the scale far.
const (
	referenceDBm   = -45.0
	maxShiftDB     = 15.0
	noSignalOffset = 0.0
)

// signalOffset is how far to shift every signal so the owner's access points read as referenceDBm at their strongest.
func signalOffset(rows []FeatureRow) float64 {
	sum, n := 0.0, 0
	for _, r := range rows {
		if r.OwnAP && r.HasMax {
			sum += float64(r.SigMax)
			n++
		}
	}
	if n == 0 {
		return noSignalOffset
	}
	return math.Max(-maxShiftDB, math.Min(maxShiftDB, referenceDBm-sum/float64(n)))
}

// vectorOf turns a device's features into the model's numbers, its signal shifted by offset (signalOffset).
func vectorOf(r Rules, f Features, kind DeviceKind, offset float64) vector {
	v := vector{hasSig: f.HasSignal}
	v.c[fShare] = f.Share
	v.c[fDays] = math.Min(1, float64(f.Days)/float64(max(1, r.WindowDays)))
	v.c[fNight], v.c[fCross] = f.NightShare, f.MoveShare
	if f.HasSignal {
		v.c[fSigMean], v.c[fSigStd], v.c[fSigMax] = f.SignalMean+offset, f.SignalStd, float64(f.SignalMax)+offset
	}
	v.b[bAP], v.b[bRandom], v.b[bOnOwn] = f.AccessPoint, f.Random, f.JoinsOwn
	v.b[bFixedKind] = fixedKinds[kind]
	return v
}

// fixedKinds are kinds of thing that stay in one place.
var fixedKinds = map[DeviceKind]bool{DeviceRouter: true, DeviceTV: true, DeviceSmartHome: true, DeviceCamera: true, DevicePrinter: true, DeviceSpeaker: true, DeviceConsole: true}

func newModel(r Rules) *Model {
	m := &Model{Version: r.Version, Stats: map[Class]*classStats{}}
	for c, p := range priorsFrom(r) {
		st := &classStats{W: priorWeight, SigW: priorWeight}
		for i := range numContinuous {
			st.Sum[i] = priorWeight * p.mean[i]
			st.Sq[i] = priorWeight * (p.sd[i]*p.sd[i] + p.mean[i]*p.mean[i])
		}
		for i := range numBinary {
			st.B[i] = priorWeight * p.p[i]
		}
		m.Stats[c] = st
	}
	return m
}

// add puts one weighted example into a class. A self-training example stops counting once the class has had selfCap of them.
func (m *Model) add(c Class, v vector, w float64, self bool) {
	st := m.Stats[c]
	if st == nil {
		return
	}
	if self {
		if st.Self >= selfCap {
			return
		}
		w = math.Min(w, selfCap-st.Self)
		st.Self += w
	}
	st.W += w
	for i := range numBinary {
		if v.b[i] {
			st.B[i] += w
		}
	}
	st.Share(v, w)
}

// Share adds the continuous numbers; the signal ones only when the example has a signal.
func (st *classStats) Share(v vector, w float64) {
	for _, i := range alwaysKnown {
		st.Sum[i] += w * v.c[i]
		st.Sq[i] += w * v.c[i] * v.c[i]
	}
	if !v.hasSig {
		return
	}
	st.SigW += w
	for _, i := range []int{fSigMean, fSigStd, fSigMax} {
		st.Sum[i] += w * v.c[i]
		st.Sq[i] += w * v.c[i] * v.c[i]
	}
}

// weightOf is the weight a continuous number of a class has seen.
func (st *classStats) weightOf(i int) float64 {
	if isSignal(i) {
		return st.SigW
	}
	return st.W
}

// alwaysKnown are the numbers every device has; the others are signal numbers, missing when no signal was measured.
var alwaysKnown = []int{fShare, fDays, fNight, fCross}

func isSignal(i int) bool { return i == fSigMean || i == fSigStd || i == fSigMax }

// logLikelihood is how well each number of v fits a class, one term per number, so a decision can be explained by its largest terms.
func (st *classStats) logLikelihood(v vector) [numContinuous + numBinary]float64 {
	var out [numContinuous + numBinary]float64
	for i := range numContinuous {
		if !v.hasSig && isSignal(i) {
			continue
		}
		w := st.weightOf(i)
		mean := st.Sum[i] / w
		variance := math.Max(minVariance, st.Sq[i]/w-mean*mean)
		d := v.c[i] - mean
		out[i] = -0.5*math.Log(2*math.Pi*variance) - d*d/(2*variance)
	}
	for i := range numBinary {
		p := (st.B[i] + 0.5) / (st.W + 1) // smoothed
		if !v.b[i] {
			p = 1 - p
		}
		out[numContinuous+i] = math.Log(p)
	}
	return out
}

// Posterior is the chance of each class for v. onOwn limits it to the classes a device on the owner's Wi-Fi can be.
func (m *Model) Posterior(v vector) map[Class]float64 {
	total := 0.0
	for _, c := range learnClasses {
		total += m.Stats[c].W
	}
	logs := map[Class]float64{}
	best := math.Inf(-1)
	for _, c := range learnClasses {
		if v.b[bOnOwn] && (c == ClassNeighbour || c == ClassTransient) {
			continue // hard evidence: a device on the owner's Wi-Fi is the house's, whatever its signal says
		}
		st := m.Stats[c]
		l := math.Log(st.W / total)
		for _, t := range st.logLikelihood(v) {
			l += t
		}
		logs[c] = l
		best = math.Max(best, l)
	}
	out := map[Class]float64{}
	sum := 0.0
	for c, l := range logs {
		out[c] = math.Exp(l - best)
		sum += out[c]
	}
	for c := range out {
		out[c] /= sum
	}
	return out
}

// top is the most likely class and its chance.
func top(p map[Class]float64) (Class, float64) {
	best, bp := ClassUnknown, 0.0
	for _, c := range learnClasses {
		if p[c] > bp {
			best, bp = c, p[c]
		}
	}
	return best, bp
}

// Factor is one number that pushed a decision, and the class it pushed toward against the runner-up.
type Factor struct {
	Feature string `json:"feature"`
	Toward  string `json:"toward"`
}

// Explain says which numbers moved v most toward its class and away from the runner-up. It is worked out when the page asks and never stored.
func (m *Model) Explain(v vector, chosen Class) []Factor {
	p := m.Posterior(v)
	second, sp := ClassUnknown, -1.0
	for _, c := range learnClasses {
		if c != chosen && p[c] > sp {
			second, sp = c, p[c]
		}
	}
	a, b := m.Stats[chosen], m.Stats[second]
	if a == nil || b == nil {
		return []Factor{}
	}
	la, lb := a.logLikelihood(v), b.logLikelihood(v)
	type term struct {
		i int
		d float64
	}
	var terms []term
	for i := range la {
		terms = append(terms, term{i, la[i] - lb[i]})
	}
	sort.Slice(terms, func(x, y int) bool { return terms[x].d > terms[y].d })
	out := []Factor{}
	for _, t := range terms {
		if t.d <= 0.2 || len(out) == maxFactors {
			break
		}
		out = append(out, Factor{Feature: featureNames[t.i], Toward: string(chosen)})
	}
	return out
}

// example is one device the model trains on.
type example struct {
	class  Class
	v      vector
	weight float64
	self   bool
	owner  bool
}

// labelOf is what a device can teach: the owner's answer, a hard-evidence case, or (weakly) the rules' own answer.
func labelOf(r Rules, row FeatureRow, f Features) (example, bool) {
	if row.Class != "" && row.ClassVer == 0 && Class(row.Class) != ClassUnknown {
		return example{class: Class(row.Class), weight: ownerWeight, owner: true}, true
	}
	if f.CyclesListened < r.MinCyclesListened || f.CyclesHeard < r.MinCyclesHeard {
		return example{}, false
	}
	if c, ok := hardEvidence(r, f); ok {
		return example{class: c, weight: evidenceWeight}, true
	}
	if c, _ := r.Classify(f); c != ClassUnknown {
		return example{class: c, weight: selfWeight, self: true}, true
	}
	return example{}, false
}

// hardEvidence is a class the frames prove more than suggest: on the owner's Wi-Fi (always here, or not), or an access point never heard strong.
func hardEvidence(r Rules, f Features) (Class, bool) {
	switch {
	case f.JoinsOwn && f.Share >= r.AlwaysShare:
		return ClassInfrastructure, true
	case f.JoinsOwn:
		return ClassPersonal, true
	case f.AccessPoint && f.HasSignal && f.SignalMax <= r.WeakDBm:
		return ClassNeighbour, true
	}
	return "", false
}

// Train builds the model from the rules and every device in the window, then checks it against each owner answer it was not shown.
func Train(r Rules, examples []example, at int64) *Model {
	m := newModel(r)
	m.TrainedAt = at
	for _, e := range examples {
		m.add(e.class, e.v, e.weight, e.self)
		switch {
		case e.owner:
			m.Owner++
		case e.self:
			m.Self++
		default:
			m.Evidence++
		}
	}
	m.checkAgainstOwner(r, examples)
	return m
}

// checkAgainstOwner counts how many owner answers the model gives back when trained without that one answer (leave one out).
func (m *Model) checkAgainstOwner(r Rules, examples []example) {
	for i, e := range examples {
		if !e.owner {
			continue
		}
		without := newModel(r)
		for j, o := range examples {
			if j != i {
				without.add(o.class, o.v, o.weight, o.self)
			}
		}
		if c, _ := top(without.Posterior(e.v)); c == e.class {
			m.Agree++
		}
		m.Checked++
	}
}

// decision is what the classifier says about one device this cycle.
type decision struct {
	class    Class
	why      Why
	conf     int
	pending  string
	pendingN int
}

// decide is the class a device gets: the owner's word stands; too little evidence is Unknown; otherwise the model's answer, with hysteresis.
func decide(r Rules, m *Model, row FeatureRow, f Features, v vector) decision {
	if f.CyclesListened < r.MinCyclesListened || f.CyclesHeard < r.MinCyclesHeard {
		return decision{class: ClassUnknown, why: WhyNeedsMore}
	}
	post := m.Posterior(v)
	if row.Owner {
		return decideYours(r, post, f)
	}
	c, p := top(post)
	conf := int(math.Round(100 * p))
	if p < confidenceFloor {
		c = ClassUnknown
	}
	cur := Class(row.Class)
	if cur == "" || cur == ClassUnknown || cur == c {
		return decision{class: c, why: whyFor(r, c, f), conf: conf}
	}
	if c == ClassUnknown { // unsure is no reason to drop a class already earned: keep it, with no change pending
		return decision{class: cur, why: whyFor(r, cur, f), conf: int(math.Round(100 * post[cur]))}
	}
	n := 1
	if row.Pending == string(c) {
		n = row.PendingN + 1
	}
	if n >= switchCycles && post[c]-post[cur] >= switchMargin {
		return decision{class: c, why: whyFor(r, c, f), conf: conf}
	}
	return decision{class: cur, why: WhyHolding, conf: int(math.Round(100 * post[cur])), pending: string(c), pendingN: n}
}

// decideYours is the class of a device the owner said is theirs: it cannot be a neighbour's or a passer-by, so it is the likelier of always here
// and comes and goes, at that share of the two. No hysteresis: the owner's word already settled the question it would guard.
func decideYours(r Rules, post map[Class]float64, f Features) decision {
	infra, personal := post[ClassInfrastructure], post[ClassPersonal]
	c, p := ClassInfrastructure, infra
	if personal > infra {
		c, p = ClassPersonal, personal
	}
	share := halfShare
	if sum := infra + personal; sum > 0 {
		share = p / sum
	}
	return decision{class: c, why: whyFor(r, c, f), conf: int(math.Round(100 * share))}
}

// whyFor is the reason word of a learned decision: the rules' own reason when they agree, else "matches what was learned".
func whyFor(r Rules, c Class, f Features) Why {
	if c == ClassUnknown {
		return WhyNoClearPattern
	}
	if rc, why := r.Classify(f); rc == c {
		return why
	}
	return WhyLearned
}
