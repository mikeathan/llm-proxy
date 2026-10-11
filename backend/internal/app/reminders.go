package app

import (
	"context"
	"slices"
	"time"

	assistantPkg "llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/reminders"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// reminderDeliverer sends reminder text through the shared connector set. Like
// connectorNotifier it is a system-side send path: the destination is the
// operator's own connector and the text is exactly what the user asked for.
type reminderDeliverer struct {
	comm *tools.CommunicationTools
}

func (d reminderDeliverer) Deliver(ctx context.Context, connector, message string) error {
	// Plain: the reminder is the user's own text and must not be parsed as markup.
	return d.comm.NotifyAllPlain(ctx, message, connector)
}

func (d reminderDeliverer) HasConnector(connector string) bool {
	return slices.Contains(d.comm.Types(), connector)
}

// reminderNotice records each delivery outcome. The persisted reminder row is the
// in-app notice (served by GET /admin/api/reminders); the log line carries the id
// and outcome but never the message, which is user content.
type reminderNotice struct{}

func (reminderNotice) Notify(_ context.Context, r reminders.Reminder, event string) {
	logging.Info("reminder", "id", r.ID, "event", event, "channel", r.Connector)
}

// defaultReminderConnector is the channel for a reminder that names none and did
// not arrive through a connector: the configured default if that connector
// exists, else the only configured connector, else "" (in-app only). With several
// connectors and no setting it does not guess.
func defaultReminderConnector(comm *tools.CommunicationTools, cfg func() models.RemindersConfig) string {
	types := comm.Types()
	if want := cfg().DefaultConnector; want != "" && slices.Contains(types, want) {
		return want
	}
	if len(types) == 1 {
		return types[0]
	}
	return ""
}

// reminderPort adapts the reminders service to the agent tools' interface.
type reminderPort struct {
	svc      *reminders.Service
	settings func() models.RemindersConfig
}

func (p reminderPort) Zone(requested string) (*time.Location, error) {
	return reminders.ResolveZone(requested, p.settings().Timezone)
}

func (p reminderPort) Schedule(ctx context.Context, spec tools.ReminderSpec) (tools.ReminderView, error) {
	// Reply where asked: with no channel named, a request that came through a
	// connector is answered on it, as long as it is still configured.
	if spec.Connector == "" && spec.Origin != "" && p.svc.HasConnector(spec.Origin) {
		spec.Connector = spec.Origin
	}
	got, err := p.svc.Create(ctx, reminders.Reminder{
		Message:   spec.Message,
		FireAt:    spec.At,
		Cron:      spec.Cron,
		Until:     spec.Until,
		Timezone:  spec.Zone,
		Connector: spec.Connector,
		CreatedBy: "assistant",
	})
	if err != nil {
		return tools.ReminderView{}, err
	}
	return p.view(got), nil
}

func (p reminderPort) Cancel(ctx context.Context, id int64) error { return p.svc.Cancel(ctx, id) }

func (p reminderPort) List(ctx context.Context) ([]tools.ReminderView, error) {
	open, err := p.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]tools.ReminderView, len(open))
	for i, r := range open {
		views[i] = p.view(r)
	}
	return views, nil
}

func (p reminderPort) view(r reminders.Reminder) tools.ReminderView {
	return tools.ReminderView{
		ID: r.ID, Message: r.Message, At: r.FireAt, Cron: r.Cron, Zone: r.Timezone,
		Channel: p.svc.Channel(r.Connector),
	}
}

// localRegistry finds the local tool registry inside the tool provider chain.
func localRegistry(tp assistantPkg.ToolProvider) *assistantPkg.LocalToolRegistry {
	mtp, ok := tp.(*assistantPkg.MultiToolProvider)
	if !ok {
		return nil
	}
	for _, p := range mtp.Providers {
		if ltr, ok := p.(*assistantPkg.LocalToolRegistry); ok {
			return ltr
		}
	}
	return nil
}

// initReminders builds the reminder service and attaches its tools. It is a no-op
// (reminders unavailable, everything else unaffected) when the database or the
// connector registry is missing. The returned service's Run loop is started by
// the app lifecycle, not here.
func (c *Container) initReminders(s *AppServices) {
	provider := s.AppCtx.DBProvider()
	comm := communicationTools(s.toolProvider)
	registry := localRegistry(s.toolProvider)
	if provider == nil || comm == nil || registry == nil {
		logging.Warn("reminders disabled: database or tool registry unavailable")
		return
	}
	store, err := reminders.NewStore(provider)
	if err != nil {
		logging.Error("reminders disabled: store failed", "error", err)
		return
	}
	settings := func() models.RemindersConfig { return s.AppCtx.GetSettings().Reminders.Effective() }
	s.Reminders = reminders.NewService(store, reminderDeliverer{comm: comm}, reminderNotice{}, reminders.SystemClock(),
		reminders.Config{DefaultConnector: func() string { return defaultReminderConnector(comm, settings) }})
	registry.WithReminders(reminderPort{svc: s.Reminders, settings: settings})
}
