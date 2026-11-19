package app

import (
	"context"
	"testing"

	"llm-proxy/internal/platform/datausage"
)

// Every name the data layout documents under the data folder is described by a core kind, so the page never shows it as unknown. A new store
// under the folder appears as its own row until it is added here (datausage.Measure).
func TestCoreStoreKinds_DescribeEveryDocumentedName(t *testing.T) {
	documented := []string{"settings.yml", "registry.json", "secrets.json", "master.key", "master.key.hash", "orchestrator.db", "orchestrator.db-wal",
		"vulnindex.json", "decoys.json", "identities.json", "devices.json", "profiles", "meta", "runs", "logs", "templates", "auth.json",
		"router-history.json", "network-history.json", "ubuntu-security.json", "destination-owners.bin", "destination-places.bin"}
	for _, name := range documented {
		found := false
		for _, k := range coreStoreKinds {
			for _, n := range k.Names {
				if n == name || (len(n) > 0 && n[len(n)-1] == '*' && len(name) >= len(n)-1 && name[:len(n)-1] == n[:len(n)-1]) {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s is not described by any core kind", name)
		}
	}
}

// Every hosted stack's files are named as the stack's own kind, whichever stack it is: a stack missing from the loop shows its database as a bare
// file name with no owner (the Reminders stack was).
func TestStackStoreKinds_NameEveryHostedStacksFiles(t *testing.T) {
	s := &AppServices{netaudit: &netauditServices{Reminders: &stackHost{stack: remindersStack()}}}
	kinds, _ := s.stackStoreKinds(context.Background())
	for _, k := range kinds {
		if k.Owner == "reminders" && len(k.Names) == 1 && k.Names[0] == "reminders.db" {
			return
		}
	}
	t.Fatalf("no kind names reminders.db for the Reminders stack: %+v", kinds)
}

func TestLevelWord(t *testing.T) {
	if levelWord(0) != datausage.LevelOK || levelWord(1) != datausage.LevelLow || levelWord(2) != datausage.LevelCritical {
		t.Error("levels")
	}
}
