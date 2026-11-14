package radio

import "math"

// Distance from signal strength (plan §5). One sensor can only say a RANGE: transmit power, antennas and walls are unknown, so the model gives a
// near and a far edge from two path-loss exponents and the page draws the range, never a point. The numbers are PROVISIONAL starting values, not
// measurements; the owner's calibration (a device of theirs stood one metre from the sensor) replaces the signal at one metre for its band.

// The rings a range falls in, by the middle of the range.
const (
	RingRoom   = "room"   // within a few metres
	RingHouse  = "house"  // inside the house
	RingNearby = "nearby" // next door or the street
	RingFar    = "far"    // too weak to place
)

// The trends of a signal over the last cycles.
const (
	TrendCloser  = "closer"
	TrendAway    = "away"
	TrendSteady  = "steady"
	TrendUnknown = "unknown"
)

const (
	trendMinSamples = 3
	trendDeltaDB    = 4.0
	minMetres       = 1.0
	maxMetres       = 200.0
)

// DistanceModel is the log-distance model: d = 10^((Ref - signal) / (10 n)).
type DistanceModel struct {
	// Ref24 and Ref5 are the signal in dBm at one metre, by band.
	Ref24 float64 `json:"ref_24_dbm"`
	Ref5  float64 `json:"ref_5_dbm"`
	// NearExp and FarExp are the path-loss exponents of the near and the far edge: a large exponent means the signal falls fast, so the same
	// reading is nearer.
	NearExp     float64 `json:"near_exponent"`
	FarExp      float64 `json:"far_exponent"`
	Calibrated  bool    `json:"calibrated"`
	Provisional bool    `json:"provisional"`
}

// DefaultDistance is the model before any calibration.
func DefaultDistance() DistanceModel {
	return DistanceModel{Ref24: -40, Ref5: -47, NearExp: 3.5, FarExp: 2.0, Provisional: true}
}

func is24(channel int) bool { return channel <= 14 }

// Ref is the signal at one metre for a channel's band.
func (m DistanceModel) Ref(channel int) float64 {
	if is24(channel) {
		return m.Ref24
	}
	return m.Ref5
}

// WithRef returns the model with the signal at one metre of a channel's band set to the measured one.
func (m DistanceModel) WithRef(channel int, dbm float64) DistanceModel {
	if is24(channel) {
		m.Ref24 = dbm
	} else {
		m.Ref5 = dbm
	}
	m.Calibrated, m.Provisional = true, false
	return m
}

func metres(ref, signal, exp float64) float64 {
	return math.Min(maxMetres, math.Max(minMetres, math.Pow(10, (ref-signal)/(10*exp))))
}

// Range is the nearest and the farthest the device can be, in metres, for a mean signal heard on a channel.
func (m DistanceModel) Range(signal float64, channel int) (near, far float64) {
	ref := m.Ref(channel)
	return metres(ref, signal, m.NearExp), metres(ref, signal, m.FarExp)
}

// RingOf places a range by its geometric middle.
func RingOf(near, far float64) string {
	mid := math.Sqrt(near * far)
	switch {
	case mid <= 4:
		return RingRoom
	case mid <= 12:
		return RingHouse
	case mid <= 40:
		return RingNearby
	}
	return RingFar
}

// TrendOf compares the newest cycles' signal with the older ones; it says nothing with too few samples.
func TrendOf(trail []float64) string {
	if len(trail) < trendMinSamples {
		return TrendUnknown
	}
	k := min(2, len(trail)/2)
	newest, older := meanOf(trail[len(trail)-k:]), meanOf(trail[:len(trail)-k])
	switch d := newest - older; {
	case d >= trendDeltaDB:
		return TrendCloser
	case d <= -trendDeltaDB:
		return TrendAway
	}
	return TrendSteady
}

func meanOf(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

// angleOf is where a device is drawn around the sensor. Direction is NOT measured with one adapter, so the angle is only a stable place on the
// ring that keeps a device from jumping about between looks.
func angleOf(tag []byte) float64 {
	var h uint32 = 2166136261
	for _, b := range tag {
		h = (h ^ uint32(b)) * 16777619
	}
	return float64(h%3600) / 10
}
