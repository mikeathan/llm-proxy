package netaudit

import (
	"slices"
	"strings"
	"testing"

	"llm-proxy/internal/platform/hostwatch"
)

// stockPrivilege is a stock Ubuntu desktop as the platform reader returns it.
func stockPrivilege() hostwatch.Privilege {
	return hostwatch.Privilege{
		Checked: true, LoginAccounts: []string{"owner", "root"}, Admins: []string{"adm:owner", "sudo:owner"},
		Modules: []string{"nf_nat", "overlay"}, ModulesRead: true, SetIDComplete: true,
	}
}

func privilegeChecks(look HostLook) []string {
	var out []string
	for _, f := range look.Findings {
		if strings.HasPrefix(f.Check, "host_") && slices.Contains(privilegeCheckNames, f.Check) {
			out = append(out, f.Check)
		}
	}
	slices.Sort(out)
	return out
}

func TestHostPrivilege_AFirstLookReportsNothingAndLaterAdditionsAreFindings(t *testing.T) {
	src := &fakeHost{privilege: stockPrivilege()}
	store := &memHostStore{}
	w := newWatcher(src, store)
	look, err := w.Look(t.Context())
	if err != nil || len(privilegeChecks(look)) != 0 {
		t.Fatalf("first look = %v, %v; want nothing", privilegeChecks(look), err)
	}
	p := stockPrivilege()
	p.LoginAccounts = append(p.LoginAccounts, "helper")
	p.Admins = append(p.Admins, "docker:helper")
	p.Modules = append(p.Modules, "rootkit_mod")
	p.SetID = []string{"/usr/local/bin/shell"}
	src.privilege = p
	look, _ = w.Look(t.Context())
	want := []string{CheckHostNewAdmin, CheckHostNewLoginAccount, CheckHostNewModule, CheckHostNewSetID}
	slices.Sort(want)
	if got := privilegeChecks(look); !slices.Equal(got, want) {
		t.Fatalf("checks = %v, want %v", got, want)
	}
	for _, f := range look.Findings {
		if f.Check == CheckHostNewAdmin && !strings.Contains(f.Evidence, "docker:helper") {
			t.Errorf("evidence %q does not name the new member", f.Evidence)
		}
	}
}

func TestHostPrivilege_ASecondRootAccountAndAPreloadAreReportedEvenOnTheFirstLook(t *testing.T) {
	p := stockPrivilege()
	p.ExtraRoot, p.Preload, p.PreloadDigest = []string{"toor"}, true, "abcdef0123456789"
	look, _ := newWatcher(&fakeHost{privilege: p}, &memHostStore{}).Look(t.Context())
	want := []string{CheckHostExtraRoot, CheckHostPreload}
	slices.Sort(want)
	if got := privilegeChecks(look); !slices.Equal(got, want) {
		t.Fatalf("checks = %v, want %v", got, want)
	}
}

func TestHostPrivilege_TheFindingsVersionChangesOnlyWhenTheSetChanges(t *testing.T) {
	src := &fakeHost{privilege: stockPrivilege()}
	w := newWatcher(src, &memHostStore{})
	_, _ = w.Look(t.Context())
	p := stockPrivilege()
	p.Modules = append(p.Modules, "usb_storage")
	src.privilege = p
	first, _ := w.Look(t.Context())
	again, _ := w.Look(t.Context())
	p.Modules = append(p.Modules, "zz_other")
	src.privilege = p
	more, _ := w.Look(t.Context())
	v := func(l HostLook) string {
		for _, f := range l.Findings {
			if f.Check == CheckHostNewModule {
				return f.Version
			}
		}
		return ""
	}
	if v(first) == "" || v(first) != v(again) || v(first) == v(more) {
		t.Fatalf("versions %q, %q, %q: an accepted finding must hold until one more module appears", v(first), v(again), v(more))
	}
}

func TestHostPrivilege_AComputerThatWasNotCheckedSaysSo(t *testing.T) {
	look, _ := newWatcher(&fakeHost{}, &memHostStore{}).Look(t.Context())
	if !look.PrivilegeUnchecked || len(privilegeChecks(look)) != 0 {
		t.Fatalf("look = %+v; a computer without account files is not checked, never fine", look)
	}
}

func TestHostPrivilege_AnIncompleteWalkOrUnreadModulesNeverLearnsOrResolves(t *testing.T) {
	src := &fakeHost{privilege: stockPrivilege()}
	store := &memHostStore{}
	w := newWatcher(src, store)
	_, _ = w.Look(t.Context())
	p := stockPrivilege()
	p.ModulesRead, p.Modules = false, nil
	src.privilege = p
	look, _ := w.Look(t.Context())
	if slices.Contains(privilegeChecks(look), CheckHostNewModule) {
		t.Fatal("an unread module list is not a list of new modules")
	}
	if !slices.Equal(store.w.Privilege.Modules, []string{"nf_nat", "overlay"}) {
		t.Fatalf("baseline modules = %v; an unread list must not replace the baseline", store.w.Privilege.Modules)
	}
}
