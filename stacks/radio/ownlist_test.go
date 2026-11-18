package radio

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"llm-proxy-stacks/sdk"
)

// fakeNetworks is the core's one list of the owner's networks (the ownnetworks port): the names, and the access points confirmed on the
// Network page by their hardware addresses, tagged under whatever purpose is asked, as the core does.
type fakeNetworks struct {
	mu        sync.Mutex
	names     []string
	confirmed []MAC
	raw       [][]byte // tags given as they are, for tests that make rows with a tag of their own
	asked     []string
}

func (f *fakeNetworks) Networks(ctx context.Context, purpose string) ([]stacksdk.OwnNetwork, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, purpose)
	out := []stacksdk.OwnNetwork{}
	for i, n := range f.names {
		on := stacksdk.OwnNetwork{Name: n, Confirmed: [][]byte{}}
		if i == 0 {
			for _, m := range f.confirmed {
				tag, _ := fakeVault{}.Tag(ctx, purpose, m[:])
				on.Confirmed = append(on.Confirmed, tag)
			}
			on.Confirmed = append(on.Confirmed, f.raw...)
		}
		out = append(out, on)
	}
	return out, nil
}

// set replaces the list.
func (f *fakeNetworks) set(names []string, confirmed ...MAC) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.names, f.confirmed = names, confirmed
}

// confirmTags confirms access points by tag in the core list the recorder reads (a fake one is made when it has none, with the name "home"),
// then reads it, as a confirmation on the Network page does.
func confirmTags(t *testing.T, r *Recorder, tags ...[]byte) *fakeNetworks {
	t.Helper()
	r.cmu.Lock()
	f, ok := r.netPort.(*fakeNetworks)
	r.cmu.Unlock()
	if !ok {
		f = &fakeNetworks{}
		r.SetNetworks(f)
	}
	f.mu.Lock()
	if len(f.names) == 0 {
		f.names = []string{"home"}
	}
	f.raw = append(f.raw, tags...)
	f.mu.Unlock()
	if err := r.SyncOwnNetworks(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f
}

// withdrawTags removes every tag given as it is from the recorder's fake core list, then reads it.
func withdrawTags(t *testing.T, r *Recorder) {
	t.Helper()
	r.cmu.Lock()
	f := r.netPort.(*fakeNetworks)
	r.cmu.Unlock()
	f.mu.Lock()
	f.raw = nil
	f.mu.Unlock()
	if err := r.SyncOwnNetworks(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// coreList points a recorder at a fake core list holding names and the confirmed access points, and reads it.
func coreList(t *testing.T, r *Recorder, names []string, confirmed ...MAC) *fakeNetworks {
	t.Helper()
	f := &fakeNetworks{}
	f.set(names, confirmed...)
	r.SetNetworks(f)
	if err := r.SyncOwnNetworks(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f
}

// The stack keeps no list of its own: the names a beacon proposes under and the access points that are the owner's come from the core's list,
// and a confirmation or a withdrawal there counts at the next read.
func TestTheOwnersNetworksComeFromTheCoresOneList(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	ctx := context.Background()
	f := coreList(t, r, []string{"home"})
	record(t, r, beacon("home"), t0)
	record(t, r, beacon("sky"), t0)
	aps, err := r.OwnAPs(ctx)
	if err != nil || len(aps) != 1 || aps[0].Name != "home" || aps[0].Confirmed {
		t.Fatalf("only the core's name proposes, and a proposal is not the owner's: %+v %v", aps, err)
	}
	f.set([]string{"home"}, macAP)
	if err := r.SyncOwnNetworks(ctx); err != nil {
		t.Fatal(err)
	}
	if aps, _ = r.OwnAPs(ctx); !aps[0].Confirmed {
		t.Fatalf("confirmed on the Network page, it is the owner's: %+v", aps)
	}
	if own, _ := r.isOwnAP(ctx, macAP); !own {
		t.Error("the owner's access point counts for the evidence")
	}
	f.set([]string{"home"})
	_ = r.SyncOwnNetworks(ctx)
	if aps, _ = r.OwnAPs(ctx); aps[0].Confirmed {
		t.Errorf("withdrawn on the Network page, it is not the owner's: %+v", aps)
	}
}

// The air report tells the core of each access point heard lately under the owner's names, with its tag and its address sealed, both under
// the purpose the stack asked the core's list for, and says which the owner confirmed on the Radio page before the lists were one.
func TestTheAirReportCarriesTheAccessPointsHeardUnderTheOwnersNames(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	ctx := context.Background()
	f := coreList(t, r, []string{"home"})
	record(t, r, beacon("home"), t0)
	if _, err := s.db.Exec(`UPDATE own_aps SET confirmed = 1`); err != nil { // as the Radio page used to
		t.Fatal(err)
	}
	got, err := r.airAPs(ctx, t0)
	if err != nil || len(got) != 1 {
		t.Fatalf("aps = %+v %v", got, err)
	}
	asked := f.asked[len(f.asked)-1]
	want, _ := fakeVault{}.Tag(ctx, asked, macAP[:])
	opened, _ := fakeVault{}.Open(ctx, asked, got[0].Sealed)
	if got[0].Name != "home" || !bytes.Equal(got[0].Tag, want) || !bytes.Equal(opened, macAP[:]) || !got[0].ConfirmedBefore {
		t.Errorf("ap = %+v; want the tag and the sealed address under the asked purpose %q, marked confirmed before", got[0], asked)
	}
	if later, _ := r.airAPs(ctx, t0.AddDate(0, 0, 3)); len(later) != 0 {
		t.Errorf("an access point not heard for days is not reported: %+v", later)
	}
}

// Without the core's list (the port not given) no name is watched, nothing is proposed and no access point is the owner's.
func TestWithoutTheCoresListNothingIsTheOwners(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	if err := r.SyncOwnNetworks(context.Background()); err != nil {
		t.Fatal(err)
	}
	record(t, r, beacon("home"), t0)
	if aps, _ := r.OwnAPs(context.Background()); len(aps) != 0 {
		t.Errorf("aps = %+v", aps)
	}
}
