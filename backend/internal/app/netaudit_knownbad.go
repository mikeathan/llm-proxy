package app

import (
	"context"
	"fmt"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/knownbad"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
)

// The list of malware host names (abuse.ch URLhaus). The operator's address for it holds their key, so it is kept in the secrets store
// (encrypted) and only ever set, cleared or used for the download. The names are matched on this computer.
const (
	knownBadSecretCategory = "netaudit"
	knownBadSecretName     = "urlhaus_list"
)

// knownBad is the switch, the list and the handlers' view of both.
type knownBad struct {
	Config  *netaudit.KnownBadConfig
	Manager *knownbad.Manager
	List    *knownBadList
}

func (s *AppServices) newKnownBad(store netaudit.PolicyStore, netTools *tools.NetworkTools) knownBad {
	file := withReset(s.AppCtx, "The downloaded list of malware host names", persistence.NewKnownBadStore(s.AppCtx.RootDir()))
	address := func() string { return s.AppCtx.Secrets().GetSecret(knownBadSecretCategory, knownBadSecretName) }
	manager, err := knownbad.NewManager(knownbad.Deps{Store: file, Open: netTools.Open, Address: address})
	if err != nil {
		logging.Warn("network known-bad names", "error", err.Error())
	}
	return knownBad{Config: netaudit.NewKnownBadConfig(store), Manager: manager, List: &knownBadList{app: s.AppCtx, manager: manager}}
}

// knownBadList is what the handlers and the module use: the list, and the saved address in the secrets store.
type knownBadList struct {
	app     *AppContext
	manager *knownbad.Manager
}

func (l *knownBadList) Status() knownbad.Status { return l.manager.Status() }

func (l *knownBadList) HasAddress() bool {
	return l.app.Secrets().GetSecret(knownBadSecretCategory, knownBadSecretName) != ""
}

func (l *knownBadList) SetAddress(address string) error {
	if err := l.app.Secrets().SetSecret(knownBadSecretCategory, knownBadSecretName, address); err != nil {
		return fmt.Errorf("failed to save the list address: %w", err)
	}
	return nil
}

func (l *knownBadList) ClearAddress() error {
	if err := l.app.Secrets().DeleteSecret(knownBadSecretCategory, knownBadSecretName); err != nil {
		return fmt.Errorf("failed to remove the list address: %w", err)
	}
	return nil
}

func (l *knownBadList) Download(ctx context.Context) error { return l.manager.Download(ctx) }

// Ready says whether the comparison can work, and when it cannot, why in plain words.
func (l *knownBadList) Ready() (bool, string) {
	switch {
	case l.manager.Status().Count == 0 && !l.HasAddress():
		return false, "no list address is saved yet"
	case l.manager.Status().Count == 0:
		return false, "the list is not downloaded yet"
	}
	return true, ""
}

// matcher is what the phone-home watch compares names with: the held list while the operator has the comparison switched on, and nothing
// otherwise.
func (k knownBad) matcher() func(string) (string, bool) {
	return func(name string) (string, bool) {
		if !k.Config.Enabled() {
			return "", false
		}
		return k.Manager.Match(name)
	}
}
