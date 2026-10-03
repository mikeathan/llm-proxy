package app

import (
	"context"
	"fmt"

	assistantPkg "llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/tools"
)

// communicationTools finds the connector registry inside the tool provider
// chain; nil when no local tool registry carries one.
func communicationTools(tp assistantPkg.ToolProvider) *tools.CommunicationTools {
	mtp, ok := tp.(*assistantPkg.MultiToolProvider)
	if !ok {
		return nil
	}
	for _, p := range mtp.Providers {
		if ltr, ok := p.(*assistantPkg.LocalToolRegistry); ok {
			return ltr.Communication
		}
	}
	return nil
}

// connectorNotifier lets the automation dispatcher deliver a report through a
// configured communication connector. It is the system-side send path: the
// agent never reaches it and the destination is the operator's own connector.
type connectorNotifier struct {
	comm *tools.CommunicationTools
}

func (n connectorNotifier) Notify(ctx context.Context, connector, message string) error {
	conn, ok := n.comm.GetByName(connector)
	if !ok {
		return fmt.Errorf("connector %q is not configured or enabled", connector)
	}
	return conn.Send(ctx, message)
}
