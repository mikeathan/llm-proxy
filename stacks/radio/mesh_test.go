package radio

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

var meshBox = MAC{0x02, 0x00, 0x00, 0x00, 0x00, 0xf0}

// fakeMesh answers a scripted signal for each device and counts the asks.
type fakeMesh struct {
	mu    sync.Mutex
	asks  int
	at    time.Time
	rssi  map[MAC]int
	guest map[MAC]bool
}

func (m *fakeMesh) Clients(ctx context.Context, purpose string) (stacksdk.MeshView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.asks++
	box, _ := fakeVault{}.Tag(ctx, purpose, meshBox[:])
	v := stacksdk.MeshView{At: m.at}
	for mac, rssi := range m.rssi {
		tag, _ := fakeVault{}.Tag(ctx, purpose, mac[:])
		v.Devices = append(v.Devices, stacksdk.MeshDevice{Tag: tag, Box: box, BoxName: "Mesh, first floor", Band: "5G", Guest: m.guest[mac], RSSI: rssi})
	}
	return v, nil
}

func (m *fakeMesh) set(at time.Time, rssi map[MAC]int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.at, m.rssi = at, rssi
}

func TestTheRoutersListShowsTheBoxAndItsSignalAndAMoveOnTheDot(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	m := &fakeMesh{guest: map[MAC]bool{mineMAC: true}}
	rec.SetMesh(m)
	cycleOf(t, rec, r.now, dataAt(neighbour, -70), dataAt(mineMAC, -75))
	m.set(r.now, map[MAC]int{neighbour: -70, mineMAC: -60})
	rec.RefreshMesh(context.Background(), r.now)
	m.set(r.now.Add(30*time.Second), map[MAC]int{neighbour: -57, mineMAC: -60})
	rec.RefreshMesh(context.Background(), r.now.Add(10*time.Second)) // too soon: not asked
	rec.RefreshMesh(context.Background(), r.now.Add(30*time.Second))
	if m.asks != 2 {
		t.Errorf("the run asks at most once every %v: %d asks", meshAskEvery, m.asks)
	}
	got := map[string]LiveDevice{}
	for _, d := range liveOf(t, r).Devices {
		got[d.ID] = d
	}
	phone := got[shortOf(neighbour)]
	if phone.Router == nil || phone.Router.Dbm != -57 || phone.Router.BoxName != "Mesh, first floor" || phone.Router.Box != shortID(mustTag(t, rec, meshBox)) {
		t.Fatalf("the dot says which box and its signal: %+v", phone.Router)
	}
	if !phone.Router.Moving || !phone.Moving {
		t.Errorf("a 13 dB change at the box is a move: %+v", phone)
	}
	guest := got[shortOf(mineMAC)]
	if guest.Router == nil || !guest.Router.Guest || guest.Mine || guest.Moving {
		t.Errorf("a guest-network device is shown as guest and never as yours: %+v", guest)
	}
	rec.ClearLive()
	if rec.RouterOf(mustTag(t, rec, neighbour)) != nil {
		t.Error("the run's end forgets the router's list")
	}
}

// The router's list changes nothing stored: no share, class, "on your Wi-Fi" evidence or track.
func TestTheRoutersListWritesNothing(t *testing.T) {
	ctx := context.Background()
	plain, _ := openTest(t, 0)
	withMesh, _ := openTest(t, 0)
	a, b := NewRecorder(fakeVault{}, plain), NewRecorder(fakeVault{}, withMesh)
	m := &fakeMesh{}
	b.SetMesh(m)
	at := t0
	for range 3 {
		cycleOf(t, a, at, dataAt(neighbour, -80))
		cycleOf(t, b, at, dataAt(neighbour, -80))
		m.set(at, map[MAC]int{neighbour: -45})
		b.RefreshMesh(ctx, at)
		at = at.Add(5 * time.Minute)
	}
	rowsA, lA := storedState(t, plain)
	rowsB, lB := storedState(t, withMesh)
	if !reflect.DeepEqual(rowsA, rowsB) || !reflect.DeepEqual(lA, lB) {
		t.Errorf("the router's list must change nothing stored:\n%+v\n%+v", rowsA, rowsB)
	}
	if rowsB[0].JoinedOwn {
		t.Error("being on the router's list is never the stored evidence of being on your Wi-Fi")
	}
}

// With the router's list on, a run waits out a cycle's idle time in 30 s steps and asks for the list after each.
func TestARunKeepsTheRoutersListFreshThroughTheIdleTime(t *testing.T) {
	r := newRig(t, twoChannels, okWindow)
	rec, _, _ := r.s.recorder(r.s.h)
	m := &fakeMesh{}
	rec.SetMesh(m)
	r.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "the first wait", func() bool { return len(r.nap) == 1 })
	if d := <-r.nap; d != meshAskEvery {
		t.Errorf("the idle time is waited in steps of %v: %v", meshAskEvery, d)
	}
	r.step <- struct{}{}
	waitFor(t, "the next step", func() bool { return len(r.nap) == 1 })
	m.mu.Lock()
	asks := m.asks
	m.mu.Unlock()
	if asks < 2 {
		t.Errorf("asked during the sweep and after the first step: %d", asks)
	}
}

// downMesh is a meshclients port whose list cannot be had: the Network module's router setup is not finished, the router is down, or the core
// is not reading it.
type downMesh struct{ asks int }

func (m *downMesh) Clients(context.Context, string) (stacksdk.MeshView, error) {
	m.asks++
	return stacksdk.MeshView{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the router's list of devices is not available")
}

// Radio does not depend on the Network module: with the router's list unavailable it is asked, the answer is dropped, and the radio hears and
// shows its devices as before, with no router reading on any dot.
func TestTheRadioWorksAsBeforeWhenTheRoutersListIsUnavailable(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	m := &downMesh{}
	rec.SetMesh(m)
	cycleOf(t, rec, r.now, dataAt(neighbour, -70))
	rec.RefreshMesh(context.Background(), r.now)
	if m.asks != 1 {
		t.Fatalf("asked %d times, want 1", m.asks)
	}
	devices := liveOf(t, r).Devices
	if len(devices) == 0 {
		t.Fatal("the radio still shows what it heard")
	}
	for _, d := range devices {
		if d.Router != nil {
			t.Errorf("no router reading without the list: %+v", d)
		}
	}
}
