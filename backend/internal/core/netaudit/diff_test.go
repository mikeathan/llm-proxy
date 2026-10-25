package netaudit

import (
	"reflect"
	"testing"
)

func finding(device string, port int, version string) Finding {
	return Finding{Device: device, Port: port, Proto: "tcp", Check: CheckOpenPort, Severity: SeverityInfo, Version: version}
}

func TestDiff(t *testing.T) {
	prev := []Finding{finding("10.0.0.1", 22, "OpenSSH 8.9"), finding("10.0.0.1", 80, ""), finding("10.0.0.2", 445, "")}
	cur := []Finding{finding("10.0.0.1", 22, "OpenSSH 9.6"), finding("10.0.0.1", 80, ""), finding("10.0.0.3", 3306, "MySQL 8")}

	got := Diff(prev, cur)
	if len(got.New) != 1 || got.New[0].Device != "10.0.0.3" {
		t.Errorf("new = %+v", got.New)
	}
	if len(got.Resolved) != 1 || got.Resolved[0].Device != "10.0.0.2" {
		t.Errorf("resolved = %+v", got.Resolved)
	}
	if len(got.Changed) != 1 || got.Changed[0].Before.Version != "OpenSSH 8.9" || got.Changed[0].After.Version != "OpenSSH 9.6" {
		t.Errorf("changed = %+v", got.Changed)
	}
	if got.Unchanged != 1 {
		t.Errorf("unchanged = %d", got.Unchanged)
	}
	if got.Empty() {
		t.Error("a delta with changes is not empty")
	}
}

func TestDiff_IsDeterministicAndNilSafe(t *testing.T) {
	a := []Finding{finding("10.0.0.9", 1, ""), finding("10.0.0.1", 1, ""), finding("10.0.0.5", 1, "")}
	b := []Finding{a[2], a[0], a[1]}
	if !reflect.DeepEqual(Diff(nil, a), Diff(nil, b)) {
		t.Error("the same findings in a different order must give the same delta")
	}
	if got := Diff(nil, nil); !got.Empty() || got.Unchanged != 0 {
		t.Errorf("empty diff = %+v", got)
	}
	if got := Diff(a, a); !got.Empty() || got.Unchanged != 3 {
		t.Errorf("identical scans = %+v", got)
	}
}
