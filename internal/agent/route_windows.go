//go:build windows

package agent

import "os/exec"

func routeAddKernel(subnet string) {
	// Kernel routing not available on Windows
}

func routeDelKernel(subnet string) {
	// Kernel routing not available on Windows
}

// DetectSubnets is not implemented on Windows.
func detectSubnets() ([]string, error) {
	return nil, nil
}

func runRouteCmd(args ...string) error {
	cmd := exec.Command("route", args...)
	return cmd.Run()
}
