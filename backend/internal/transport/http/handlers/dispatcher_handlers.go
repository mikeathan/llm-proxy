package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/eventbus"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Sentinels returned from MutateConfig closures so the handler — never the
// closure — decides the HTTP response.
var (
	errAutomationExists   = errors.New("automation already exists")
	errAutomationNotFound = errors.New("automation not found")
)

// respondAutomationMutationError maps an automation config-mutation failure to
// its HTTP status: 409 for a duplicate name, 404 for a missing automation,
// 500 for anything else (lock, read or write failures).
func respondAutomationMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAutomationExists):
		respondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, errAutomationNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	default:
		respondError(w, http.StatusInternalServerError, err.Error())
	}
}

var validIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func validateID(id string) bool {
	return validIDRegex.MatchString(id)
}

// isUnsafeFileParam reports whether s is empty, ".", not a local path (absolute,
// or escaping via ".."), or normalizes differently under filepath.Clean
// (traversal or separator tricks). Nested paths such as "sub/task.md" are
// allowed. This is the syntactic first line only: workspace file operations are
// also contained in an *os.Root (persistence), which stops symlink escapes.
func isUnsafeFileParam(s string) bool {
	return s == "" || s == "." || !filepath.IsLocal(s) || filepath.Clean(s) != s
}

type Dispatcher interface {
	Persistence() *persistence.WorkspaceManager
	Register(workspaceID string, auto *models.Automation) error
	Unregister(workspaceID string, automationName string) error
	ListAll() []*automation.AutomationEntry
	Trigger(workspaceID string, automationName string, recordingRef string) (automation.TriggerResult, error)
	CancelQueued(workspaceID string, automationName string) error
	LaneSnapshot() runlane.Snapshot
	StopAutomation(workspaceID string) error
	Metrics() *automation.DispatcherMetrics
	Events() *eventbus.Bus
	UnregisterWorkspace(workspaceID string)
	HeartbeatState(workspaceID string) (models.HeartbeatState, error)
	NextRuns() map[string]time.Time
}

type DispatcherHandlers struct {
	dispatcher Dispatcher
	workspace  WorkspaceService
	logger     logging.Logger
	profiles   ProfileCatalog // nil: profile ids are not checked
}

func NewDispatcherHandlers(d Dispatcher, ws WorkspaceService, logger logging.Logger) *DispatcherHandlers {
	return &DispatcherHandlers{dispatcher: d, workspace: ws, logger: logger}
}

// WithProfiles makes saved profile choices (a workspace's, an automation's) be checked against the catalog.
func (h *DispatcherHandlers) WithProfiles(catalog ProfileCatalog) *DispatcherHandlers {
	h.profiles = catalog
	return h
}

func (h *DispatcherHandlers) parse(w http.ResponseWriter, r *http.Request, keys ...string) (wsID string, autoName string, ok bool) {
	for _, k := range keys {
		val := r.PathValue(k)
		if val == "" {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("%s is required", k))
			return "", "", false
		}
		if !validateID(val) {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid %s", k))
			return "", "", false
		}

		if k == models.WorkspaceIDParam {
			wsID = val
		}
		if k == "automation" {
			autoName = val
		}
	}
	return wsID, autoName, true
}

func (h *DispatcherHandlers) validateAutomation(auto *models.Automation) error {
	if auto == nil {
		return fmt.Errorf("automation data is required")
	}
	if !validateID(auto.Name) {
		return fmt.Errorf("invalid automation name in payload")
	}
	if auto.Name == models.HeartbeatAutomationName {
		return fmt.Errorf("the name %q is reserved: set the heartbeat up in the workspace's Heartbeat section", auto.Name)
	}
	// task_file is joined into the workspace path and its content becomes the
	// LLM prompt — reject empty/absolute/traversing values so a crafted
	// automation cannot read arbitrary files outside the workspace.
	if auto.TaskFile == "" || filepath.IsAbs(auto.TaskFile) || strings.Contains(auto.TaskFile, "..") {
		return fmt.Errorf("invalid task_file in payload")
	}
	if auto.LoopStrategy != "" && !assistant.LoopStrategyName(auto.LoopStrategy).Valid() {
		return fmt.Errorf("invalid loop_strategy %q: valid values are %s",
			auto.LoopStrategy, strings.Join(assistant.RegisteredLoopStrategyNames(), ", "))
	}
	return validateRunOptions(auto)
}

// checkProfileChange refuses an unknown profile id only when the request is setting it: the id the config already
// stores stays acceptable even if its file has since been deleted, because the whole-document PUT carries every stored
// value back and one missing profile must not block an unrelated save (the same rule as validateSchedulable).
func (h *DispatcherHandlers) checkProfileChange(field, stored, proposed string) error {
	if proposed == stored {
		return nil
	}
	return validateProfileID(h.profiles, field, proposed)
}

// validateAutomationProfile checks an automation's profile against the config as stored, which may be nil.
func (h *DispatcherHandlers) validateAutomationProfile(auto *models.Automation, storedAs string, stored *models.WorkspaceConfig) error {
	var current string
	if stored != nil {
		for _, a := range stored.Automations {
			if strings.TrimSpace(a.Name) == storedAs {
				current = a.Profile
			}
		}
	}
	return h.checkProfileChange("profile", current, auto.Profile)
}

// validateSchedulable refuses a trigger the scheduler cannot read, using the
// scheduler's own factory so the two can never disagree. Create and update
// call it so a bad schedule is told to the user instead of saved. The
// whole-document config PUT does not: it carries every stored automation back,
// and one older unreadable trigger must not block an unrelated save (the
// dispatcher skips that automation and schedules the rest).
func validateSchedulable(auto *models.Automation) error {
	if _, err := automation.New(auto.Trigger); err != nil {
		return fmt.Errorf("invalid trigger: %w", err)
	}
	return nil
}

// validateMemoryMode accepts an explicit override or empty (inherit the global default).
func validateMemoryMode(field string, mode models.MemoryMode) error {
	if mode != models.MemoryModeInherit && !mode.Valid() {
		return fmt.Errorf("invalid %s %q: valid values are on, off (empty = inherit the global default)", field, mode)
	}
	return nil
}

// validateRunOptions fail-fast checks the optional per-run settings.
func validateRunOptions(auto *models.Automation) error {
	if err := validateMemoryMode("memory_mode", auto.MemoryMode); err != nil {
		return err
	}
	// network_grant is an explicit per-run scope override; empty = inherit the
	// workspace scope. Reject unknown values fail-fast (sandboxing plan §4.4).
	if auto.NetworkGrant != "" && !auto.NetworkGrant.Valid() {
		return fmt.Errorf("invalid network_grant %q: valid values are none, lan, internet_only, internet (empty = inherit)", auto.NetworkGrant)
	}
	if auto.Notify != nil {
		if err := auto.Notify.Validate(); err != nil {
			return fmt.Errorf("invalid notify: %w", err)
		}
	}
	return nil
}

type AutomationInfo struct {
	ID           string `json:"id"`
	Workspace    string `json:"workspace"`
	Name         string `json:"name"`
	TaskFile     string `json:"task_file"`
	Strategy     string `json:"strategy"`
	Trigger      string `json:"trigger"`
	TriggerValue string `json:"trigger_value,omitempty"`
	Model        string `json:"model,omitempty"`
	LoopStrategy string `json:"loop_strategy,omitempty"`
	RecordingRef string `json:"recording_ref,omitempty"`
	NetworkGrant string `json:"network_grant,omitempty"` // '' = inherit workspace scope
	MemoryMode   string `json:"memory_mode,omitempty"`   // '' = off
	Profile      string `json:"profile,omitempty"`       // '' = the default for automations
	LastOutput   string `json:"last_output,omitempty"`
	LastError    string `json:"last_error,omitempty"`
	// LastWarnings are the latest run's non-fatal problems, e.g. a report that could not be delivered.
	LastWarnings []string `json:"last_warnings,omitempty"`
	// LastReportPath is the workspace path of the latest run's saved final report, for a link to the file.
	LastReportPath string `json:"last_report_path,omitempty"`
	// NextRunAt is when the scheduler will next fire it (server clock, RFC 3339); absent for a manual automation.
	NextRunAt time.Time `json:"next_run_at,omitzero"`
	IsRunning bool      `json:"is_running"`
	// Queued/QueuePosition report the run scheduler admission: the automation
	// is waiting in its workload lane (Position is 1-based). Both are empty
	// while the run executes or sits idle — IsRunning stays the source of
	// truth for "actually executing".
	Queued        bool                   `json:"queued,omitempty"`
	QueuePosition int                    `json:"queue_position,omitempty"`
	History       []models.AutomationRun `json:"history,omitempty"`

	// Notify is the automation's result-delivery config; nil = none.
	Notify *models.NotifyConfig `json:"notify,omitempty"`
	// SkipIfBusy: scheduled fires are skipped, not queued, while the lane is busy.
	SkipIfBusy bool `json:"skip_if_busy,omitempty"`
	// Journal: the automation keeps a learning journal (read it with GetAutomationJournal).
	Journal bool `json:"journal,omitempty"`
}

func (h *DispatcherHandlers) ListAutomations(w http.ResponseWriter, r *http.Request) {
	entries := h.dispatcher.ListAll()
	infos := make([]AutomationInfo, 0, len(entries))

	// One snapshot per request feeds every queued badge (position must come
	// from the admission-time state, never a per-entry re-read).
	queuedBy := make(map[string]runlane.Entry)
	for _, lane := range h.dispatcher.LaneSnapshot().Lanes {
		for _, e := range lane.Queued {
			queuedBy[e.Key] = e
		}
	}

	nextRuns := h.dispatcher.NextRuns()

	for _, entry := range entries {
		info := AutomationInfo{
			ID:           entry.ID,
			Workspace:    entry.Workspace,
			Name:         entry.Name,
			Trigger:      string(entry.Trigger.Type()),
			TriggerValue: entry.Trigger.Value(),
			TaskFile:     entry.TaskFile,
			Strategy:     entry.Strategy.Name(),
			Model:        entry.Model,
			LoopStrategy: string(entry.LoopStrategy),
			RecordingRef: entry.RecordingRef,
			NetworkGrant: string(entry.NetworkGrant),
			MemoryMode:   string(entry.MemoryMode),
			Profile:      entry.Profile,
			Notify:       entry.Notify,
			SkipIfBusy:   entry.SkipIfBusy,
			Journal:      entry.Journal,
			NextRunAt:    nextRuns[entry.ID],
		}

		if state, err := h.workspace.GetState(entry.Workspace); err == nil {
			for _, run := range state.History {
				if run.AutomationName == entry.Name {
					info.History = append(info.History, run)
				}
			}

			// An automation's output is sourced only from its own latest run
			// (LastRuns). The workspace-wide LastOutput/LastError fields are
			// legacy and never fall back to here, so a run-less automation
			// reports no stale output.
			if last, ok := state.LastRuns[entry.Name]; ok {
				info.LastOutput = last.Output
				info.LastError = last.Error
				info.LastWarnings = last.Warnings
				info.LastReportPath = last.ReportPath
			}
			info.IsRunning = state.ActiveAutomation == entry.Name
		}
		if e, ok := queuedBy[entry.ID]; ok {
			info.Queued = true
			info.QueuePosition = e.Position
		}

		infos = append(infos, info)
	}

	respondJSON(w, infos)
}

func (h *DispatcherHandlers) TriggerAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID, automationName, ok := h.parse(w, r, models.WorkspaceIDParam, "automation")
	if !ok {
		return
	}

	recordingRef := r.URL.Query().Get("recording_ref")

	// Submit is a cheap lane enqueue (the run timeout starts at dequeue), so
	// the trigger runs inline and reports the admission disposition. The old
	// 409-on-running path is gone: overlapping runs queue instead.
	result, err := h.dispatcher.Trigger(workspaceID, automationName, recordingRef)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	status := http.StatusOK
	if result.Status == automation.TriggerQueued {
		status = http.StatusAccepted
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     result.Status,
		"position":   result.Position,
		"workspace":  workspaceID,
		"automation": automationName,
	})
}

func (h *DispatcherHandlers) StopAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}

	if err := h.dispatcher.StopAutomation(workspaceID); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, map[string]string{
		"status":    "stopped",
		"workspace": workspaceID,
	})
}

func (h *DispatcherHandlers) GetDispatcherMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := h.dispatcher.Metrics().Snapshot()

	respondJSON(w, map[string]interface{}{
		"total_executions": metrics.TotalExecutions,
		"successful":       metrics.SuccessfulExecutions,
		"failed":           metrics.FailedExecutions,
		"skipped":          metrics.SkippedExecutions,
		"queued":           metrics.QueuedExecutions,
		"preempted":        metrics.PreemptedExecutions,
		"total_latency_ms": metrics.TotalLatency.Milliseconds(),
	})
}

// CancelQueuedHandler handles DELETE /admin/api/dispatcher/queue/{workspace}/
// {automation}: it drops a queued (not yet running) entry. Stopping a running
// automation is POST /dispatcher/stop/{workspace}, which also clears pending.
func (h *DispatcherHandlers) CancelQueuedHandler(w http.ResponseWriter, r *http.Request) {
	workspaceID, automationName, ok := h.parse(w, r, models.WorkspaceIDParam, "automation")
	if !ok {
		return
	}

	if err := h.dispatcher.CancelQueued(workspaceID, automationName); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *DispatcherHandlers) GetWorkspaceState(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	state, err := h.workspace.GetState(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, struct {
		*models.AgentState
		IsRunning bool `json:"is_running"`
	}{
		AgentState: state,
		IsRunning:  state.IsRunning(),
	})
}

func (h *DispatcherHandlers) GetWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	cfg, err := h.workspace.GetConfig(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, cfg)
}

// validateWorkspaceConfig checks a whole-document config before anything is persisted: the memory and profile
// choices, the heartbeat, and every automation (this endpoint once bypassed validateAutomation, letting a crafted
// task_file or name escape the workspace path and read arbitrary files into the LLM prompt).
func (h *DispatcherHandlers) validateWorkspaceConfig(cfg, stored *models.WorkspaceConfig) error {
	if err := validateMemoryMode("assistant_memory", cfg.AssistantMemory); err != nil {
		return err
	}
	var storedProfile string
	if stored != nil {
		storedProfile = stored.AssistantProfile
	}
	if err := h.checkProfileChange("assistant_profile", storedProfile, cfg.AssistantProfile); err != nil {
		return err
	}
	if cfg.Heartbeat != nil {
		if err := cfg.Heartbeat.Validate(); err != nil {
			return err
		}
	}
	for _, auto := range cfg.Automations {
		if err := errors.Join(h.validateAutomation(auto), h.validateAutomationProfile(auto, strings.TrimSpace(auto.Name), stored)); err != nil {
			return err
		}
	}
	return nil
}

func (h *DispatcherHandlers) UpdateWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}

	var cfg models.WorkspaceConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	stored, _ := h.workspace.GetConfig(workspaceID) // unreadable: nothing is stored to tolerate, so every id is checked
	if err := h.validateWorkspaceConfig(&cfg, stored); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Acquire lock, read existing, merge, write, release — handled atomically by WorkspaceService.
	if err := h.workspace.MutateConfig(workspaceID, func(existing *models.WorkspaceConfig) error {
		// Merge logic: If automations aren't in the request, keep the old ones
		if cfg.Automations == nil {
			cfg.Automations = existing.Automations
		}
		*existing = cfg
		return nil
	}); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, auto := range cfg.Automations {
		if err := h.dispatcher.Register(workspaceID, auto); err != nil {
			h.logger.Error("failed to register automation from config",
				"workspace", workspaceID, "automation", auto.Name, "error", err)
		}
	}

	respondJSON(w, map[string]string{"status": "updated"})
}

// requestedStream is the event stream a /live request asks for. Assistant chat asks
// for ?channel=assistant. An automation run's console asks for ?automation=<name>:
// events are routed per automation, so it only ever sees its own automation's run,
// never another's in the same workspace. ok is false when that name is missing.
func requestedStream(r *http.Request) (channel assistant.EventChannel, ok bool) {
	channel = assistant.EventChannel(r.URL.Query().Get("channel"))
	if channel != "" && channel != assistant.ChannelAutomation {
		return channel, true
	}
	automation := r.URL.Query().Get("automation")
	return assistant.AutomationStream(automation), automation != ""
}

func (h *DispatcherHandlers) StreamWorkspaceEvents(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}

	channel, ok := requestedStream(r)
	if !ok {
		respondError(w, http.StatusBadRequest, "automation is required for the automation stream")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ch, recent := h.dispatcher.Events().Subscribe(workspaceID, channel)
	defer h.dispatcher.Events().Unsubscribe(workspaceID, channel, ch)

	// Context for cancellation
	ctx := r.Context()

	// Initial ping
	fmt.Fprintf(w, "event: ping\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	// Replay recent events for current run
	for _, ev := range recent {
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: agent_update\ndata: %s\n\n", string(data))
	}
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: agent_update\ndata: %s\n\n", string(data))
			flusher.Flush()
		}
	}
}

// GetGlobalActivity returns the most recent automation runs across every
// workspace, read from the persisted run history (the same source the run
// delete endpoints purge).
func (h *DispatcherHandlers) GetGlobalActivity(w http.ResponseWriter, r *http.Request) {
	runs, err := h.workspace.RecentRuns(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, runs)
}

func (h *DispatcherHandlers) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := h.workspace.ListWorkspaces()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(workspaces))
	for _, ws := range workspaces {
		result = append(result, map[string]interface{}{
			"id": ws.ID,
		})
	}
	respondJSON(w, result)
}

func (h *DispatcherHandlers) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.ID == "" {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}

	if !validateID(req.ID) {
		respondError(w, http.StatusBadRequest, "invalid workspace ID (must be alphanumeric/hyphen)")
		return
	}

	// Create default task files in workspace root
	defaultTaskFiles := map[string]string{
		models.HeartbeatFilename: prompts.DefaultHeartbeat,
		models.RulesFilename:     prompts.DefaultAgentsMD,
	}

	if err := h.workspace.CreateWorkspace(req.ID, &models.WorkspaceConfig{
		Temperature: 0.7,
	}, defaultTaskFiles); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "created", "id": req.ID})
}

// ListWorkspaceTree returns the workspace's bounded recursive file tree.
func (h *DispatcherHandlers) ListWorkspaceTree(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	tree, err := h.workspace.ListTree(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, tree)
}

func (h *DispatcherHandlers) ReadWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	filename := r.PathValue("file")
	if isUnsafeFileParam(filename) {
		respondError(w, http.StatusBadRequest, "invalid file path")
		return
	}

	content, err := h.workspace.ReadTaskFile(workspaceID, filename)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, map[string]string{"content": content})
}

func (h *DispatcherHandlers) WriteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	filename := r.PathValue("file")
	if isUnsafeFileParam(filename) {
		respondError(w, http.StatusBadRequest, "invalid file path")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := h.workspace.WriteTaskFile(workspaceID, filename, req.Content); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "saved"})
}

func (h *DispatcherHandlers) UpdateAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID, automationName, ok := h.parse(w, r, models.WorkspaceIDParam, "automation")
	if !ok {
		return
	}

	var auto models.Automation
	if err := json.NewDecoder(r.Body).Decode(&auto); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	stored, _ := h.workspace.GetConfig(workspaceID) // unreadable: nothing is stored to tolerate
	if err := errors.Join(h.validateAutomation(&auto), h.validateAutomationProfile(&auto, automationName, stored), validateSchedulable(&auto)); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.workspace.MutateConfig(workspaceID, func(cfg *models.WorkspaceConfig) error {
		for i, a := range cfg.Automations {
			if strings.TrimSpace(a.Name) == automationName {
				newAuto := auto
				cfg.Automations[i] = &newAuto
				return nil
			}
		}

		var names []string
		for _, a := range cfg.Automations {
			names = append(names, fmt.Sprintf("'%s'", a.Name))
		}
		h.logger.Warn("UpdateAutomation failed: automation not found",
			"workspace", workspaceID,
			"target", automationName,
			"available", strings.Join(names, ", "))
		return errAutomationNotFound
	}); err != nil {
		respondAutomationMutationError(w, err)
		return
	}

	if automationName != auto.Name {
		h.dispatcher.Unregister(workspaceID, automationName)
	}

	regAuto := auto
	if err := h.dispatcher.Register(workspaceID, &regAuto); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "updated"})
}

func (h *DispatcherHandlers) CreateAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}

	var auto models.Automation
	if err := json.NewDecoder(r.Body).Decode(&auto); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := errors.Join(h.validateAutomation(&auto), h.validateAutomationProfile(&auto, "", nil), validateSchedulable(&auto)); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.workspace.MutateConfig(workspaceID, func(cfg *models.WorkspaceConfig) error {
		// Ensure no duplicate name
		for _, a := range cfg.Automations {
			if strings.TrimSpace(a.Name) == strings.TrimSpace(auto.Name) {
				return errAutomationExists
			}
		}
		// Create a persistent copy in config
		newAuto := auto
		cfg.Automations = append(cfg.Automations, &newAuto)
		return nil
	}); err != nil {
		respondAutomationMutationError(w, err)
		return
	}

	// Register in memory
	regAuto := auto
	if err := h.dispatcher.Register(workspaceID, &regAuto); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "created"})
}

func (h *DispatcherHandlers) DeleteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	filename := r.PathValue("file")
	if isUnsafeFileParam(filename) {
		respondError(w, http.StatusBadRequest, "invalid file path")
		return
	}

	recursive, err := recursiveParam(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// ?recursive=true removes a folder with its content; without it a folder
	// must be empty, as before.
	if recursive {
		err = h.workspace.DeleteTaskTree(r.Context(), workspaceID, filename)
	} else {
		err = h.workspace.DeleteTaskFile(workspaceID, filename)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}

// recursiveParam reads the optional ?recursive= flag of a file delete; absent
// means false, anything strconv.ParseBool rejects is a client error.
func recursiveParam(r *http.Request) (bool, error) {
	raw := r.URL.Query().Get("recursive")
	if raw == "" {
		return false, nil
	}
	recursive, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid recursive flag %q: %w", raw, err)
	}
	return recursive, nil
}

func (h *DispatcherHandlers) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}

	// Stop any in-flight automation before deleting the workspace's on-disk
	// state, so a running executor cannot keep writing to a removed meta/ or
	// runs/ tree. "No active run" is not an error — the workspace may be idle.
	if err := h.dispatcher.StopAutomation(workspaceID); err != nil && !errors.Is(err, automation.ErrNoActiveRun) {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Unregister its automations first so the scheduler stops firing them
	h.dispatcher.UnregisterWorkspace(workspaceID)

	if err := h.workspace.DeleteWorkspace(workspaceID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}

func (h *DispatcherHandlers) DeleteAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID, automationName, ok := h.parse(w, r, models.WorkspaceIDParam, "automation")
	if !ok {
		return
	}

	var found bool
	if err := h.workspace.MutateConfig(workspaceID, func(cfg *models.WorkspaceConfig) error {
		for i, a := range cfg.Automations {
			if a.Name == automationName {
				cfg.Automations = append(cfg.Automations[:i], cfg.Automations[i+1:]...)
				found = true
				break
			}
		}
		return nil
	}); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Always try to unregister from dispatcher (handles "ghost" entries).
	h.dispatcher.Unregister(workspaceID, automationName)

	// Remove the per-automation runs tree and purge its history so deleting an
	// automation (even a "ghost" entry missing from config) does not leave
	// orphaned run dirs/recordings behind.
	if err := h.workspace.DeleteAutomationRuns(workspaceID, automationName); err != nil {
		h.logger.Warn("failed to remove automation runs dir", "workspace", workspaceID, "automation", automationName, "error", err)
	}

	status := "deleted"
	if !found {
		status = "deleted/cleaned"
	}
	respondJSON(w, map[string]string{"status": status})
}

// DeleteRun removes a single automation run by its history ID so individual
// runs can be pruned from the UI. The run is looked up in state.json by ID, so
// deletion works uniformly for every run (including runs without an on-disk
// directory). The run ID is a generated token (run_<nano>) and is never joined
// into a filesystem path, so only empty/dot segments are rejected for it while
// the workspace ID is fully validated.
func (h *DispatcherHandlers) DeleteRun(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	runID := r.PathValue("run")

	if !validateID(workspaceID) || isUnsafeFileParam(runID) {
		respondError(w, http.StatusBadRequest, "invalid run path")
		return
	}

	if err := h.workspace.DeleteRunByID(workspaceID, runID); err != nil {
		if errors.Is(err, persistence.ErrRunNotFound) {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}

// DeleteAutomationRuns removes every run directory for an automation across all
// model subdirs and purges the matching history from state.json, so a user can
// clear an automation's entire runs folder from the UI.
// GetAutomationJournal returns the automation's learning journal; an automation
// that never wrote one has an empty journal.
func (h *DispatcherHandlers) GetAutomationJournal(w http.ResponseWriter, r *http.Request) {
	workspaceID, automation, ok := h.parse(w, r, models.WorkspaceIDParam, "automation")
	if !ok {
		return
	}
	journal, err := h.workspace.ReadJournal(workspaceID, automation)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"journal": journal})
}

// ClearAutomationJournal erases the automation's learning journal so its next
// run starts without notes.
func (h *DispatcherHandlers) ClearAutomationJournal(w http.ResponseWriter, r *http.Request) {
	workspaceID, automation, ok := h.parse(w, r, models.WorkspaceIDParam, "automation")
	if !ok {
		return
	}
	if err := h.workspace.DeleteJournal(workspaceID, automation); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "cleared"})
}

func (h *DispatcherHandlers) DeleteAutomationRuns(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	automation := r.PathValue("automation")

	if !validateID(workspaceID) || !validateID(automation) {
		respondError(w, http.StatusBadRequest, "invalid automation path")
		return
	}

	if err := h.workspace.DeleteAutomationRuns(workspaceID, automation); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The console replays the latest run's steps from memory: they go with the runs.
	h.dispatcher.Events().Clear(workspaceID, assistant.AutomationStream(automation))
	respondJSON(w, map[string]string{"status": "deleted"})
}

// schedulePreviewRequest is the body of a schedule preview: the trigger to read
// and how many upcoming runs to list (0 = the default).
type schedulePreviewRequest struct {
	Trigger models.TriggerConfig `json:"trigger"`
	Count   int                  `json:"count"`
}

// PreviewSchedule answers "would this schedule work, and when would it run?"
// with the scheduler's own parser. It is stateless: nothing is registered or
// written, and an invalid schedule is a 200 carrying the reason.
func (h *DispatcherHandlers) PreviewSchedule(w http.ResponseWriter, r *http.Request) {
	var req schedulePreviewRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	respondJSON(w, automation.PreviewSchedule(req.Trigger, time.Now(), req.Count))
}
