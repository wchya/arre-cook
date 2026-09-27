package video

import (
	"os"
	"strconv"
	"strings"
)

func memoryField(path, field string) (int64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[0] == field {
			value, err := strconv.ParseInt(parts[1], 10, 64)
			return value * 1024, err == nil && value >= 0
		}
	}
	return 0, false
}

func localAvailableMemory() (int64, bool) {
	available, ok := memoryField("/proc/meminfo", "MemAvailable:")
	if !ok {
		return 0, false
	}
	// Respect cgroup v2 as well as host headroom when an operator sets a limit.
	limitRaw, limitErr := os.ReadFile("/sys/fs/cgroup/memory.max")
	currentRaw, currentErr := os.ReadFile("/sys/fs/cgroup/memory.current")
	if limitErr == nil && currentErr == nil {
		limit, err1 := strconv.ParseInt(strings.TrimSpace(string(limitRaw)), 10, 64)
		current, err2 := strconv.ParseInt(strings.TrimSpace(string(currentRaw)), 10, 64)
		if err1 == nil && err2 == nil {
			available = min(available, max(0, limit-current))
		}
	}
	return available, true
}

func localProcessRSS(pid int) int64 {
	value, _ := memoryField("/proc/"+strconv.Itoa(pid)+"/status", "VmRSS:")
	return value
}
