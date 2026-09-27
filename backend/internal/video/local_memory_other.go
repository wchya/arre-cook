//go:build !linux

package video

func localAvailableMemory() (int64, bool) { return 0, false }
func localProcessRSS(pid int) int64       { return 0 }
