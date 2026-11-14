package radio

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"llm-proxy-stacks/sdk"
)

// The scan plan the operator sets (channels, window, cycle). A change is validated here and refused with a sentence rather than trimmed in
// silence; Settings.Plan still applies the same caps when a run starts, so a document written by hand cannot exceed them either. A new plan
// takes effect at the next start, because a session approves the exact list of windows the plan makes.
const (
	// ScanPath reads (GET) and sets (PUT) the scan plan.
	ScanPath = "/admin/api/radio/listening/settings"

	minWindowSeconds = 1
	maxWindowSeconds = 30
	minCycleSeconds  = 60
)

// ScanLimits is what the settings may be, so the page offers only what will be accepted.
type ScanLimits struct {
	Channels          []int `json:"channels"`
	MaxChannels       int   `json:"max_channels"`
	MinWindowSeconds  int   `json:"min_window_seconds"`
	MaxWindowSeconds  int   `json:"max_window_seconds"`
	MinCycleSeconds   int   `json:"min_cycle_seconds"`
	MaxWindowsPerHour int   `json:"max_windows_per_hour"`
}

// ScanView is the answer of ScanPath.
type ScanView struct {
	// Plan is what a run would do now; Default is what it does with nothing set; Custom says the operator has set something.
	Plan           Plan       `json:"plan"`
	Default        Plan       `json:"default"`
	Custom         bool       `json:"custom"`
	WindowsPerHour int        `json:"windows_per_hour"`
	Limits         ScanLimits `json:"limits"`
}

// scanBody is a PUT: all three fields at once. An empty list and zero seconds mean "the default", which is how the plan is reset.
type scanBody struct {
	Channels      []int `json:"channels"`
	WindowSeconds int   `json:"window_seconds"`
	CycleSeconds  int   `json:"cycle_seconds"`
}

func (p Plan) windowsPerHour() int { return len(p.Channels) * 3600 / max(p.CycleSeconds, 1) }

func scanView(set Settings) ScanView {
	plan := set.Plan()
	return ScanView{
		Plan: plan, Default: Settings{}.Plan(), WindowsPerHour: plan.windowsPerHour(),
		Custom: len(set.Channels) > 0 || set.WindowSeconds > 0 || set.CycleSeconds > 0,
		Limits: ScanLimits{
			Channels: validChannels, MaxChannels: maxChannels, MinWindowSeconds: minWindowSeconds, MaxWindowSeconds: maxWindowSeconds,
			MinCycleSeconds: minCycleSeconds, MaxWindowsPerHour: MaxWindowsPerHour,
		},
	}
}

// checkScan says in a sentence what is wrong with a requested plan, or "" when it is acceptable.
func checkScan(b scanBody) string {
	if msg := checkScanFields(b); msg != "" {
		return msg
	}
	plan := Settings{Channels: b.Channels, WindowSeconds: b.WindowSeconds, CycleSeconds: b.CycleSeconds}.Plan()
	if len(b.Channels) > 0 && len(plan.Channels) < len(b.Channels) {
		return fmt.Sprintf("that is more than %d windows an hour: choose fewer channels or a longer cycle", MaxWindowsPerHour)
	}
	if len(plan.Channels)*plan.WindowSeconds > plan.CycleSeconds {
		return "the windows do not fit in one cycle: shorten them, choose fewer channels or a longer cycle"
	}
	return ""
}

// checkScanFields checks each field alone: the channels the helper accepts, once each, and the window and cycle inside their bounds.
func checkScanFields(b scanBody) string {
	if len(b.Channels) > maxChannels {
		return fmt.Sprintf("choose at most %d channels", maxChannels)
	}
	seen := map[int]bool{}
	for _, c := range b.Channels {
		if !slices.Contains(validChannels, c) {
			return fmt.Sprintf("channel %d cannot be listened on", c)
		}
		if seen[c] {
			return fmt.Sprintf("channel %d is listed twice", c)
		}
		seen[c] = true
	}
	if b.WindowSeconds != 0 && (b.WindowSeconds < minWindowSeconds || b.WindowSeconds > maxWindowSeconds) {
		return fmt.Sprintf("a window lasts %d to %d seconds", minWindowSeconds, maxWindowSeconds)
	}
	if b.CycleSeconds != 0 && b.CycleSeconds < minCycleSeconds {
		return fmt.Sprintf("a cycle is at least %d seconds", minCycleSeconds)
	}
	return ""
}

func (s *Stack) handleScan(ctx context.Context, h stacksdk.Host, req stacksdk.Request) (stacksdk.Response, error) {
	if req.Method == http.MethodGet {
		return jsonResponse(http.StatusOK, scanView(s.settings(ctx, h)))
	}
	var b scanBody
	if err := strictDecode(req.Body, &b); err != nil {
		return plainError(http.StatusBadRequest, "send channels, window_seconds and cycle_seconds")
	}
	if msg := checkScan(b); msg != "" {
		return plainError(http.StatusBadRequest, msg)
	}
	if h.Config == nil {
		return plainError(http.StatusConflict, "the settings are not available to the Radio stack")
	}
	channels := append([]int{}, b.Channels...)
	slices.Sort(channels)
	doc, _ := jsonBytes(map[string]any{"channels": channels, "window_seconds": b.WindowSeconds, "cycle_seconds": b.CycleSeconds})
	if err := h.Config.Save(ctx, doc); err != nil {
		return plainError(http.StatusInternalServerError, "the plan could not be saved")
	}
	return jsonResponse(http.StatusOK, scanView(s.settings(ctx, h)))
}
