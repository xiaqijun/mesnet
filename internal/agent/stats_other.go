//go:build !linux

package agent

func readCPU() float64    { return 0 }
func readMemMB() uint64   { return 0 }
