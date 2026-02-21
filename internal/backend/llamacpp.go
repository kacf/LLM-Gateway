package backend

import (
	"archive/zip"
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
	resp, err := http.Get("https://api.github.com/repos/ggerganov/llama.cpp/releases/latest")
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

	target := targetAssetHint()

	// Exact match first
	for _, a := range release.Assets {
		lower := strings.ToLower(a.Name)
		if strings.Contains(lower, target) && strings.HasSuffix(lower, ".zip") {
			return a.URL, a.Name, nil
		}
	}

	// Broad fallback — match OS, exclude CUDA/Vulkan (CPU only)
	for _, a := range release.Assets {
		lower := strings.ToLower(a.Name)
		if strings.Contains(lower, runtime.GOOS) &&
			strings.HasSuffix(lower, ".zip") &&
			!strings.Contains(lower, "cuda") &&
			!strings.Contains(lower, "vulkan") &&
			!strings.Contains(lower, "sycl") {
			return a.URL, a.Name, nil
		}
	}

	return "", "", fmt.Errorf("no llama.cpp release found for %s/%s", runtime.GOOS, runtime.GOARCH)
}

func targetAssetHint() string {
	switch {
	case runtime.GOOS == "windows" && runtime.GOARCH == "amd64":
		return "win-avx2-x64"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return "macos-arm64"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return "macos-x64"
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return "linux-x64"
	default:
		return runtime.GOOS + "-" + runtime.GOARCH
	}
}

func (m *Manager) extractBinaries(zipPath, serverDest string) error {
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
		return fmt.Errorf("llama-server not found in zip")
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
