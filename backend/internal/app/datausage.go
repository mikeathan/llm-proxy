package app

import (
	"context"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/datausage"
	"llm-proxy/internal/platform/hostwatch"
	"llm-proxy/internal/platform/sizewatch"
)

// The data management page (plan cross-cutting/data-management-page.md, D1 and D2): every top-level entry of the data folder as one row of
// its kind, the stacks' files with what each stack says of them, the workspace folders as a size only, and the volume's free space. It only
// reads; deleting stays where it was (the Clean words point there).

// coreStoreKinds names the core's own stores under the data folder, by what they are for. Anything else under the folder is listed under its
// own name (datausage.Measure), so a new store cannot hide.
var coreStoreKinds = []datausage.Kind{
	{ID: "settings", Title: "Settings, keys and sign-in", Owner: "core", Clean: "A factory reset puts the settings, the registry, the secrets and the key back to their defaults (Settings → Security).",
		Names: []string{"settings.yml", "registry.json", "secrets.json", "master.key", "master.key.hash", "auth.json", "server.json", "lockdown.json"}},
	{ID: "database", Title: "Memory and ledger database", Owner: "core", Names: []string{"orchestrator.db*"}},
	{ID: "meta", Title: "Conversations and workspace state", Owner: "core", Names: []string{"meta"},
		Clean: "Delete a conversation from its menu; Clear runtime data removes every conversation (Settings → Security)."},
	{ID: "runs", Title: "Run history and recordings", Owner: "core", Names: []string{"runs"},
		Clean: "Delete a run on Activity or a recording on Recordings; Clear runtime data removes them all (Settings → Security)."},
	{ID: "logs", Title: "Logs", Owner: "core", Names: []string{"logs"}, Clean: "Clear runtime data (Settings → Security)."},
	{ID: "profiles", Title: "Your prompt profiles", Owner: "core", Names: []string{"profiles"}},
	{ID: "downloads", Title: "Downloaded data tables", Owner: "core", Clean: "Most can be removed on Network → Settings → Data.",
		Names: []string{"vulnindex.json", "ubuntu-security.json", "destination-owners.bin", "destination-places.bin", "known-bad-names.json"}},
	{ID: "network", Title: "Network module records", Owner: "core", Clean: "A factory reset removes the device list, the fingerprints and the decoy record (Settings → Security).",
		Names: []string{"devices.json", "device-*", "identities.json", "decoys.json", "nearby.json", "phone-home.json", "router-history.json",
			"hostwatch.json", "footprint-results.json", "testssl.json", "network-*"}},
	{ID: "templates", Title: "Old playbook folder (no longer read; it can be deleted)", Owner: "core", Names: []string{"templates"}},
}

// storageSizes measures folders at most once a minute.
var storageSizes = sizewatch.NewCache(0)

// StorageReport is the data management page's report.
func (s *AppServices) StorageReport(ctx context.Context) (datausage.Report, error) {
	root := s.AppCtx.RootDir()
	now := time.Now()
	kinds, uses := s.stackStoreKinds(ctx)
	rows, err := datausage.Measure(root, append(kinds, coreStoreKinds...), storageSizes.Bytes, now)
	if err != nil {
		return datausage.Report{}, err
	}
	withStackUse(rows, uses)
	r := datausage.Report{Root: root, Rows: rows, Total: datausage.Total(rows), Level: datausage.LevelUnknown}
	if ws := s.AppCtx.WorkspacesDir(); ws != "" {
		r.Workspaces = &datausage.Row{ID: "workspaces", Title: "Workspace folders (your own work: size only)", Owner: "people",
			Entries: []string{ws}, Bytes: storageSizes.Bytes(ws, now), Described: true}
	}
	if free, size, err := hostwatch.VolumeSpace(root); err == nil {
		r.Free, r.Size, r.Level = free, size, levelWord(hostwatch.DiskLevel(free, size))
	}
	return r, nil
}

func levelWord(l int) string {
	switch l {
	case hostwatch.DiskCritical:
		return datausage.LevelCritical
	case hostwatch.DiskLow:
		return datausage.LevelLow
	}
	return datausage.LevelOK
}

// stackStoreKinds names each hosted stack's files (Manifest.Storage) as a kind of its own, and collects what the stacks that report their
// storage say of each file (stacksdk.StorageReporter), keyed by file name. A stack that cannot answer still has its files named.
func (s *AppServices) stackStoreKinds(ctx context.Context) ([]datausage.Kind, map[string]stacksdk.StorageUse) {
	var kinds []datausage.Kind
	uses := map[string]stacksdk.StorageUse{}
	if s.netaudit == nil {
		return nil, uses
	}
	for _, h := range s.netaudit.hosts() {
		m := h.stack.Manifest()
		for _, f := range m.Storage {
			kinds = append(kinds, datausage.Kind{ID: "stack:" + m.ID + ":" + f, Title: m.Title + " stack: " + f, Owner: m.ID, Names: []string{f}})
		}
		rep, ok := h.stack.(stacksdk.StorageReporter)
		if !ok {
			continue
		}
		list, err := rep.StorageUse(ctx)
		if err != nil {
			continue
		}
		for _, u := range list {
			uses[u.File] = u
		}
	}
	return kinds, uses
}

// withStackUse adds a stack's own words about a file (its cap, records, oldest record and how long it keeps one) to the file's row.
func withStackUse(rows []datausage.Row, uses map[string]stacksdk.StorageUse) {
	for i := range rows {
		if len(rows[i].Entries) != 1 {
			continue
		}
		if u, ok := uses[rows[i].Entries[0]]; ok {
			rows[i].CapBytes, rows[i].Records, rows[i].Oldest, rows[i].KeepFor = u.CapBytes, u.Rows, u.Oldest, u.KeepFor
		}
	}
}
