package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"

	"llm-proxy/models"
)

func hostOf(addr string, up, unscanned bool, ports ...int) Host {
	h := Host{Addr: netip.MustParseAddr(addr), Up: up, Unscanned: unscanned}
	for _, n := range ports {
		h.Ports = append(h.Ports, Port{Number: n, Proto: "tcp", State: "open"})
	}
	return h
}

func expect(device string, ports ...int) models.Expectation {
	return models.Expectation{Device: device, Ports: ports}
}

func TestExpectationFindings_ReportsWhatDiffersFromTheList(t *testing.T) {
	hosts := []Host{hostOf("192.168.50.10", true, false, 22, 631, 9100)}
	got := ExpectationFindings([]models.Expectation{expect("192.168.50.10", 22, 80)}, hosts)

	want := map[string]string{
		"192.168.50.10|tcp|631|unexpected_port":     SeverityMedium,
		"192.168.50.10|tcp|9100|unexpected_port":    SeverityMedium,
		"192.168.50.10|tcp|80|expected_port_closed": SeverityLow,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d findings %+v, want %d", len(got), got, len(want))
	}
	for _, f := range got {
		sev, ok := want[f.Key()]
		if !ok || f.Severity != sev {
			t.Errorf("unexpected finding %q severity %q", f.Key(), f.Severity)
		}
		if f.Evidence == "" {
			t.Errorf("%q has no evidence", f.Key())
		}
	}
}

func TestExpectationFindings_NothingToReportWhenTheDeviceMatches(t *testing.T) {
	hosts := []Host{hostOf("192.168.50.10", true, false, 631)}
	if got := ExpectationFindings([]models.Expectation{expect("192.168.50.10", 631)}, hosts); len(got) != 0 {
		t.Fatalf("a matching device produced %+v", got)
	}
}

func TestExpectationFindings_AnEmptyListMeansNothingShouldAnswer(t *testing.T) {
	hosts := []Host{hostOf("192.168.50.10", true, false, 22)}
	got := ExpectationFindings([]models.Expectation{{Device: "192.168.50.10", Ports: []int{}}}, hosts)
	if len(got) != 1 || got[0].Check != CheckUnexpectedPort || got[0].Port != 22 {
		t.Fatalf("got %+v, want one unexpected_port on 22", got)
	}
}

func TestExpectationFindings_SkipsDevicesItCannotJudge(t *testing.T) {
	exps := []models.Expectation{expect("192.168.50.10", 22), expect("192.168.50.11", 22), expect("192.168.50.12", 22)}
	hosts := []Host{
		hostOf("192.168.50.10", true, true),   // the scan ran out of time: its ports are unknown, not none
		hostOf("192.168.50.11", false, false), // down: not answering says nothing about its ports
		// .12 was not in the scan at all
	}
	if got := ExpectationFindings(exps, hosts); len(got) != 0 {
		t.Fatalf("judged a device it could not see: %+v", got)
	}
}

func TestExpectationFindings_IgnoresPortsThatAreNotOpenTCP(t *testing.T) {
	h := hostOf("192.168.50.10", true, false, 22)
	h.Ports = append(h.Ports, Port{Number: 80, Proto: "tcp", State: "closed"}, Port{Number: 53, Proto: "udp", State: "open"})
	got := ExpectationFindings([]models.Expectation{expect("192.168.50.10", 22)}, []Host{h})
	if len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}

func TestExpectationFindings_IsDeterministic(t *testing.T) {
	hosts := []Host{hostOf("192.168.50.10", true, false, 9100, 631), hostOf("192.168.50.2", true, false, 23)}
	exps := []models.Expectation{expect("192.168.50.10", 80, 22), expect("192.168.50.2")}
	a, b := ExpectationFindings(exps, hosts), ExpectationFindings(slices.Clone(exps), hosts)
	if len(a) != 5 || !slices.EqualFunc(a, b, func(x, y Finding) bool { return x.Key() == y.Key() }) {
		t.Fatalf("not stable or wrong count: %+v vs %+v", a, b)
	}
}

func TestNewExpectation_ValidatesAndNormalises(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	e, err := NewExpectation("192.168.050.010", []int{631, 22, 631}, " the printer ", now)
	if err == nil {
		t.Fatalf("a device with leading zeros should be refused, got %+v", e)
	}
	e, err = NewExpectation("192.168.50.10", []int{631, 22, 631}, " the printer ", now)
	if err != nil {
		t.Fatal(err)
	}
	if e.Device != "192.168.50.10" || !slices.Equal(e.Ports, []int{22, 631}) || e.Note != "the printer" || !e.SetAt.Equal(now) {
		t.Fatalf("not normalised: %+v", e)
	}
	empty, err := NewExpectation("192.168.50.10", nil, "", now)
	if err != nil || empty.Ports == nil || len(empty.Ports) != 0 {
		t.Fatalf("an empty list is allowed and kept as an empty list, got %+v, %v", empty, err)
	}
}

func TestNewExpectation_RefusesBadInput(t *testing.T) {
	now := time.Now()
	tooMany := make([]int, maxExpectedPorts+1)
	for i := range tooMany {
		tooMany[i] = i + 1
	}
	cases := map[string]struct {
		device string
		ports  []int
		note   string
	}{
		"not an address": {"printer.local", []int{22}, ""},
		"ipv6":           {"fe80::1", []int{22}, ""},
		"port zero":      {"192.168.50.10", []int{0}, ""},
		"port too big":   {"192.168.50.10", []int{70000}, ""},
		"too many ports": {"192.168.50.10", tooMany, ""},
		"note too long":  {"192.168.50.10", []int{22}, string(make([]rune, maxNoteLen+1))},
	}
	for name, c := range cases {
		if _, err := NewExpectation(c.device, c.ports, c.note, now); !errors.Is(err, ErrBadExpectation) {
			t.Errorf("%s: err = %v, want ErrBadExpectation", name, err)
		}
	}
}

type fakeExpectationStore struct {
	items []models.Expectation
	err   error
}

func (f *fakeExpectationStore) ReadExpectations(string) ([]models.Expectation, error) {
	return slices.Clone(f.items), f.err
}
func (f *fakeExpectationStore) WriteExpectations(_ string, items []models.Expectation) error {
	f.items = slices.Clone(items)
	return nil
}

type fakeTargets struct{ problems []TargetProblem }

func (f fakeTargets) CheckTargets(context.Context, []string) []TargetProblem { return f.problems }

func TestExpectations_SetReplacesListsAndRemoves(t *testing.T) {
	store := &fakeExpectationStore{}
	svc := NewExpectations(store, fakeTargets{})
	ctx := context.Background()

	if _, err := svc.Set(ctx, "w", "192.168.50.10", []int{22}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Set(ctx, "w", "192.168.50.10", []int{22, 631}, "printer"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.List("w")
	if len(got) != 1 || !slices.Equal(got[0].Ports, []int{22, 631}) {
		t.Fatalf("set should replace the device's entry, got %+v", got)
	}
	if ok, err := svc.Remove("w", "192.168.50.10"); !ok || err != nil {
		t.Fatalf("Remove = %v, %v", ok, err)
	}
	if ok, _ := svc.Remove("w", "192.168.50.10"); ok {
		t.Fatal("removing a device with no entry should say so")
	}
}

func TestExpectations_RefusesADeviceOutsideTheScope(t *testing.T) {
	svc := NewExpectations(&fakeExpectationStore{}, fakeTargets{problems: []TargetProblem{{Entry: "192.168.50.10", Reason: reasonOutside}}})
	_, err := svc.Set(context.Background(), "w", "192.168.50.10", []int{22}, "")
	if !errors.Is(err, ErrBadExpectation) {
		t.Fatalf("err = %v, want ErrBadExpectation naming the reason", err)
	}
}

func TestExpectations_CapsTheNumberOfDevices(t *testing.T) {
	store := &fakeExpectationStore{}
	for i := 0; i < maxExpectedDevices; i++ {
		store.items = append(store.items, expect(netip.AddrFrom4([4]byte{10, 0, byte(i / 256), byte(i % 256)}).String(), 22))
	}
	svc := NewExpectations(store, fakeTargets{})
	if _, err := svc.Set(context.Background(), "w", "192.168.50.10", []int{22}, ""); !errors.Is(err, ErrBadExpectation) {
		t.Fatalf("err = %v, want a refusal at the device cap", err)
	}
}
