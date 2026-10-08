//go:build darwin

package process

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func listByBinary(binaryName string, activePID int) ([]Info, error) {
	out, err := exec.Command("ps", "-eo", "pid,lstart,command").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	var result []Info

	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid == 0 {
			continue
		}

		dateStr := strings.Join(fields[1:6], " ")
		started, err := time.ParseInLocation(time.ANSIC, dateStr, time.Local)
		if err != nil {
			continue
		}

		command := strings.Join(fields[6:], " ")
		if command == "" {
			continue
		}

		binary, rest, ok := splitExecutable(command, binaryName)
		if !ok {
			continue
		}

		model, port := parseArgs(rest)

		result = append(result, Info{
			PID:     pid,
			Binary:  binary,
			Model:   model,
			Port:    port,
			Started: started,
			Uptime:  uptime(started),
			Active:  pid == activePID,
		})
	}

	return result, nil
}
