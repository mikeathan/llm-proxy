package port

import (
	"context"
	"errors"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/session"
)

// SessionUser counts one use of a bounded session. grant.Sessions satisfies it.
type SessionUser interface {
	Use(stack, action string, canonical []byte) error
}

// SessionPort is the Privileged port of one stack that may run actions under a bounded session (Constitution I.17). A request that carries a
// grant takes the grant path unchanged. A request with none is served only while an open session of this stack lists exactly this action and
// these checked parameters, the action is still switched on, and the action is one the catalogue marks standing; otherwise it is refused as a
// request without a valid grant is. The stack never holds a token: the session is the core's, and the stack's name is fixed where the port is
// built, so a stack cannot use another's session. Every limit of the plain port (the target scope, the hourly cap) still applies, and the helper
// still checks its own three gates.
type SessionPort struct {
	port     *Port
	sessions SessionUser
	stack    string
	on       func(action string) bool
}

// ForStack returns the port a stack is given. on says whether an action is switched on in the host settings.
func (p *Port) ForStack(stack string, sessions SessionUser, on func(action string) bool) *SessionPort {
	return &SessionPort{port: p, sessions: sessions, stack: stack, on: on}
}

// Run implements stacksdk.Privileged.
func (s *SessionPort) Run(ctx context.Context, req stacksdk.PrivilegedRequest) (stacksdk.PrivilegedResult, error) {
	if req.Grant != "" {
		return s.port.Run(ctx, req)
	}
	return s.port.run(ctx, req, func(a catalog.Action, prepared catalog.Prepared) error {
		switch {
		case !a.Standing:
			return stacksdk.Errorf(stacksdk.CodeDenied, msgGrant)
		case s.on == nil || !s.on(a.ID):
			return stacksdk.Errorf(stacksdk.CodeDenied, msgOff)
		}
		if err := s.sessions.Use(s.stack, a.ID, prepared.Canonical); err != nil {
			if errors.Is(err, session.ErrNotCovered) {
				return stacksdk.Errorf(stacksdk.CodeDenied, msgSession)
			}
			return stacksdk.Errorf(stacksdk.CodeInternal, msgFailed)
		}
		return nil
	})
}

var _ stacksdk.Privileged = (*SessionPort)(nil)
