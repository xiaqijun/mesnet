package agent

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"

	"github.com/mesnet/mesnet/internal/version"
)

const ReleaseURL = "https://meshnet.kisectool.com"

// SelfUpdate downloads the latest agent binary, replaces itself, and exits.
func SelfUpdate() error {
	arch := runtime.GOARCH
	goos := runtime.GOOS

	binName := fmt.Sprintf("mesnet-agent-%s-%s", goos, arch)
	url := fmt.Sprintf("%s/%s", ReleaseURL, binName)

	log.Printf("self-update: downloading %s", url)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Download to temp file first
	tmpPath := "/tmp/mesnet-agent.new"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("copy: %w", err)
	}
	f.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod: %w", err)
	}

	// Move new binary into place
	exePath, err := os.Executable()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("find self: %w", err)
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace: %w", err)
	}

	log.Printf("self-update: replaced %s, exiting for restart", exePath)

	// Exit — systemd will restart us with the new binary
	os.Exit(0)

	return nil // unreachable
}

// CheckVersion returns the current agent version.
func CheckVersion() string {
	return AgentVersion
}
