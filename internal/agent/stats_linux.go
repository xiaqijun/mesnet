//go:build linux

package agent

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	prevCPUTotal  uint64
	prevCPUIdle   uint64
	prevCPUTime   time.Time
	cpuMu         sync.Mutex
)

// readCPU reads /proc/stat and returns CPU usage percentage since last call.
func readCPU() float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	// Parse first line: "cpu  user nice system idle iowait irq softirq steal ..."
	line := strings.SplitN(string(data), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}

	// Sum all CPU times
	var total, idle uint64
	for i := 1; i < len(fields); i++ {
		v, _ := strconv.ParseUint(fields[i], 10, 64)
		total += v
		if i == 4 { // idle is field 5 (index 4)
			idle = v
		}
	}
	// iowait is field 6 (index 5), also considered idle
	if len(fields) > 6 {
		if iowait, err := strconv.ParseUint(fields[5], 10, 64); err == nil {
			idle += iowait
		}
	}

	cpuMu.Lock()
	defer cpuMu.Unlock()

	now := time.Now()
	if prevCPUTime.IsZero() {
		prevCPUTotal = total
		prevCPUIdle = idle
		prevCPUTime = now
		return 0
	}

	totalDelta := total - prevCPUTotal
	idleDelta := idle - prevCPUIdle
	prevCPUTotal = total
	prevCPUIdle = idle

	if totalDelta == 0 {
		return 0
	}
	return float64(totalDelta-idleDelta) / float64(totalDelta) * 100
}

// readMemMB reads /proc/meminfo and returns total memory in MB.
func readMemMB() uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			total, _ := parseKB(line)
			return total / 1024
		}
	}
	return 0
}

func parseKB(line string) (uint64, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, nil
	}
	v, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}
