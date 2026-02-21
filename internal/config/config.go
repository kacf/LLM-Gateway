package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	AppName     = "llmgw"
	Version     = "1.0.0"
	DefaultPort = 8080
	DefaultCtx  = 4096
	BackendPort = 39741
)

// Config holds all application configuration.
type Config struct {
	HomeDir     string
	ModelsDir   string
	BinDir      string
	Port        int
	CtxSize     int
	BackendPort int
	Verbose     bool
	Quant       string
}

// New creates a Config with sensible defaults.
func New() *Config {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	appDir := filepath.Join(home, "."+AppName)

	return &Config{
		HomeDir:     appDir,
		ModelsDir:   filepath.Join(appDir, "models"),
		BinDir:      filepath.Join(appDir, "bin"),
		Port:        DefaultPort,
		CtxSize:     DefaultCtx,
		BackendPort: BackendPort,
	}
}

// EnsureDirs creates all required directories.
func (c *Config) EnsureDirs() error {
	for _, d := range []string{c.HomeDir, c.ModelsDir, c.BinDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

// BackendBinaryPath returns the path to the llama-server binary.
func (c *Config) BackendBinaryPath() string {
	name := "llama-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(c.BinDir, name)
}

// ModelDir returns the cache directory for a specific model repo.
func (c *Config) ModelDir(repoID string) string {
	safe := filepath.Join(c.ModelsDir, sanitize(repoID))
	return safe
}

func sanitize(s string) string {
	out := make([]byte, len(s))
	for i := range s {
		if s[i] == '/' || s[i] == '\\' || s[i] == ':' {
			out[i] = '_'
		} else {
			out[i] = s[i]
		}
	}
	return string(out)
}
