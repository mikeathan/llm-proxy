//go:build linux

package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func listByBinary(binaryName string, activePID int) ([]Info, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}

	var out []Info
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == 0 {
			continue
		}

		binary, args, err := readCmdline(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}

		if !strings.Contains(filepath.Base(binary), binaryName) {
			continue
		}

		info := Info{
			PID:    pid,
			Binary: binary,
			Active: pid == activePID,
		}

		info.Model, info.Port = parseArgs(args)

		if st, err := os.Stat(filepath.Join("/proc", e.Name())); err == nil {
			info.Started = st.ModTime()
			info.Uptime = uptime(info.Started)
		}

		out = append(out, info)
	}

	return out, nil
}
