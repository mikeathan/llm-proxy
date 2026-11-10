//go:build darwin

package hostwatch

import (
	"net"
	"os"
	"slices"
	"testing"

	"golang.org/x/sys/unix"
)

// The real kernel on this Mac: a port this test opens is found, and is left out when it is this program's own.
func TestSystemListeners_FindsARealListenerAndLeavesOutTheProgramsOwn(t *testing.T) {
	l, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	want := Listener{"tcp", l.Addr().(*net.TCPAddr).Port}
	tcp, err := unix.SysctlRaw("net.inet.tcp.pcblist_n")
	if err != nil {
		t.Fatal(err)
	}
	udp, err := unix.SysctlRaw("net.inet.udp.pcblist_n")
	if err != nil {
		t.Fatal(err)
	}
	others, err := listenersFromPCB(tcp, udp, -1, 49152)
	if err != nil || !slices.Contains(others, want) {
		t.Fatalf("the listener %+v is not in %+v (%v)", want, others, err)
	}
	mine, err := listenersFromPCB(tcp, udp, os.Getpid(), 49152)
	if err != nil || slices.Contains(mine, want) {
		t.Errorf("a port this program holds is never news: %+v (%v)", mine, err)
	}
	got, err := systemListeners()
	if err != nil || slices.Contains(got, want) {
		t.Errorf("systemListeners must read the same and leave out its own: %+v (%v)", got, err)
	}
}

func TestSource_ListenersFallsBackToTheSystemTablesWhereThereIsNoProc(t *testing.T) {
	got, err := Source{Proc: Proc{Root: t.TempDir()}}.Listeners()
	if err != nil {
		t.Fatalf("a Mac can list its ports without /proc: %v", err)
	}
	if got == nil {
		t.Error("an answer, possibly empty, is expected")
	}
}
