package isolationtest

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"
)

// The probe is the program the suite runs inside the confinement: the test binary itself, started with these variables. It tries
// one thing and prints one line, "OK <detail>" or "DENIED <error>", so the suite learns what the confinement allowed.
const (
	actionVar = "ISOTEST_ACTION"
	argVar    = "ISOTEST_ARG"
)

// Probe actions.
const (
	actionRead   = "read"  // read the file named by the argument
	actionWrite  = "write" // create the file named by the argument
	actionTCP    = "tcp"   // connect to the host:port named by the argument
	actionUnix   = "unix"  // make a Unix socket
	actionEnv    = "env"   // list the names of the environment variables
	actionFSize  = "fsize" // report the per-file size limit
	probeTimeout = 2 * time.Second
)

// RunProbeIfRequested makes the process act as the probe when the suite started it as one. Call it from TestMain after
// isolation.RunChildIfRequested. It exits the process on the probe path and returns on a normal test start.
func RunProbeIfRequested() {
	action := os.Getenv(actionVar)
	if action == "" {
		return
	}
	fmt.Println(probe(action, os.Getenv(argVar)))
	os.Exit(0)
}

func probe(action, arg string) string {
	handlers := map[string]func(string) (string, error){
		actionRead: probeRead, actionWrite: probeWrite, actionTCP: probeTCP,
		actionUnix: probeUnix, actionEnv: probeEnv, actionFSize: probeFSize,
	}
	h, ok := handlers[action]
	if !ok {
		return "DENIED unknown probe action " + action
	}
	detail, err := h(arg)
	if err != nil {
		return "DENIED " + err.Error()
	}
	return "OK " + detail
}

func probeRead(path string) (string, error) {
	b, err := os.ReadFile(path)
	return fmt.Sprintf("%d bytes", len(b)), err
}

func probeWrite(path string) (string, error) {
	return "", os.WriteFile(path, []byte("probe"), 0o600)
}

func probeTCP(addr string) (string, error) {
	c, err := net.DialTimeout("tcp", addr, probeTimeout)
	if err != nil {
		return "", err
	}
	return "", c.Close()
}

func probeUnix(string) (string, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return "", err
	}
	return "", syscall.Close(fd)
}

func probeEnv(string) (string, error) {
	keys := make([]string, 0, 8)
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ","), nil
}

func probeFSize(string) (string, error) {
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", lim.Cur), nil
}
