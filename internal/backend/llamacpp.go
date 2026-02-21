package backend

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/llmgw/llmgw/internal/config"
	"github.com/llmgw/llmgw/internal/downloader"
	"github.com/llmgw/llmgw/internal/ui"
)

// Manager handles the llama.cpp server lifecycle.
type Manager struct {
	cfg *config.Config
	cmd *exec.Cmd
}

// New creates a backend manager.
func New(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg}
}

// EnsureBackend downloads the llama.cpp server binary if it doesn't exist.
func (m *Manager) EnsureBackend() error {
	binPath := m.cfg.BackendBinaryPath()
	if _, err := os.Stat(binPath); err == nil {
		return nil
	}

	ui.Info("Downloading llama.cpp inference server...")

	dlURL, assetName, err := m.findRelease()
	if err != nil {
		return fmt.Errorf("finding llama.cpp release: %w", err)
	}

	zipPath := filepath.Join(m.cfg.BinDir, assetName)
	if err := downloader.DownloadFile(dlURL, zipPath, "llama.cpp"); err != nil {
		return fmt.Errorf("downloading llama.cpp: %w", err)
	}

	if err := m.extractBinaries(zipPath, binPath); err != nil {
		return fmt.Errorf("extracting llama.cpp: %w", err)
	}

	os.Remove(zipPath)

	if runtime.GOOS != "windows" {
		os.Chmod(binPath, 0755)
	}

	ui.Success("llama.cpp server installed")
	return nil
}

// Start launches llama-server with the given model.
func (m *Manager) Start(modelPath string) error {
	binPath := m.cfg.BackendBinaryPath()
	args := []string{
		"-m", modelPath,
		"--port", fmt.Sprintf("%d", m.cfg.BackendPort),
		"-c", fmt.Sprintf("%d", m.cfg.CtxSize),
		"--host", "127.0.0.1",
	}

	m.cmd = exec.Command(binPath, args...)
	m.cmd.Dir = m.cfg.BinDir

	if m.cfg.Verbose {
		m.cmd.Stdout = os.Stdout
		m.cmd.Stderr = os.Stderr
	} else {
		m.cmd.Stdout = io.Discard
		m.cmd.Stderr = io.Discard
	}

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("starting llama-server: %w", err)
	}
	return nil
}

// WaitReady polls the backend health endpoint until it responds OK.
func (m *Manager) WaitReady(timeout time.Duration) error {
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", m.cfg.BackendPort)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		// Check if process died
		if m.cmd.ProcessState != nil && m.cmd.ProcessState.Exited() {
			return fmt.Errorf("llama-server process exited unexpectedly")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("backend not ready after %v", timeout)
}

// Stop kills the backend process.
func (m *Manager) Stop() {
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
		m.cmd.Wait()
	}
}

// BackendURL returns the internal backend base URL.
func (m *Manager) BackendURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", m.cfg.BackendPort)
}

// ------- internal helpers -------

func (m *Manager) findRelease() (downloadURL, assetName string, err error) {
	resp, err := http.Get("https://api.github.com/repos/ggml-org/llama.cpp/releases/latest")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var release struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", err
	}

	targets := targetAssetHints()

	// Try each hint in priority order
	for _, target := range targets {
		for _, a := range release.Assets {
			lower := strings.ToLower(a.Name)
			if strings.Contains(lower, target) && isArchive(lower) {
				return a.URL, a.Name, nil
			}
		}
	}

	// Broad fallback — match OS keyword, exclude accelerators (CPU only)
	osKey := osAssetKey()
	for _, a := range release.Assets {
		lower := strings.ToLower(a.Name)
		if strings.Contains(lower, osKey) &&
			isArchive(lower) &&
			!strings.Contains(lower, "cuda") &&
			!strings.Contains(lower, "vulkan") &&
			!strings.Contains(lower, "sycl") &&
			!strings.Contains(lower, "hip") &&
			!strings.Contains(lower, "opencl") &&
			!strings.Contains(lower, "aclgraph") &&
			!strings.Contains(lower, "cudart") {
			return a.URL, a.Name, nil
		}
	}

	return "", "", fmt.Errorf("no llama.cpp release found for %s/%s", runtime.GOOS, runtime.GOARCH)
}

// targetAssetHints returns prioritized asset name substrings to match.
func targetAssetHints() []string {
	switch {
	case runtime.GOOS == "windows" && runtime.GOARCH == "amd64":
		return []string{"win-cpu-x64", "win-avx2-x64"}
	case runtime.GOOS == "windows" && runtime.GOARCH == "arm64":
		return []string{"win-cpu-arm64", "win-arm64"}
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return []string{"macos-arm64"}
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return []string{"macos-x64"}
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return []string{"ubuntu-x64"}
	default:
		return []string{runtime.GOOS + "-" + runtime.GOARCH}
	}
}

// osAssetKey returns the OS keyword used in llama.cpp asset names.
func osAssetKey() string {
	switch runtime.GOOS {
	case "windows":
		return "win-"
	case "darwin":
		return "macos-"
	case "linux":
		return "ubuntu-"
	default:
		return runtime.GOOS
	}
}

// isArchive checks if the filename is a supported archive format.
func isArchive(name string) bool {
	return strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar.gz")
}

func (m *Manager) extractBinaries(archivePath, serverDest string) error {
	if strings.HasSuffix(strings.ToLower(archivePath), ".tar.gz") {
		return m.extractFromTarGz(archivePath, serverDest)
	}
	return m.extractFromZip(archivePath, serverDest)
}

func (m *Manager) extractFromZip(zipPath, serverDest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()

	serverName := "llama-server"
	if runtime.GOOS == "windows" {
		serverName = "llama-server.exe"
	}

	found := false
	for _, f := range r.File {
		base := filepath.Base(f.Name)

		if base == serverName {
			if err := extractZipFile(f, serverDest); err != nil {
				return err
			}
			found = true
			continue
		}

		// Also extract DLLs on Windows (needed at runtime)
		if runtime.GOOS == "windows" && strings.HasSuffix(strings.ToLower(base), ".dll") {
			dest := filepath.Join(m.cfg.BinDir, base)
			extractZipFile(f, dest) // best-effort
		}
	}

	if !found {
		return fmt.Errorf("llama-server not found in archive")
	}
	return nil
}

func (m *Manager) extractFromTarGz(tarGzPath, serverDest string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return fmt.Errorf("opening tar.gz: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	serverName := "llama-server"

	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		base := filepath.Base(hdr.Name)
		if base == serverName {
			out, err := os.Create(serverDest)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return err
			}
			found = true
		}
	}

	if !found {
		return fmt.Errorf("llama-server not found in archive")
	}
	return nil
}

func extractZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}
