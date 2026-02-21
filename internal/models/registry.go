package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/llmgw/llmgw/internal/config"
)

// Built-in short-name aliases for popular GGUF model repos.
var Aliases = map[string]string{
	"tinyllama":   "TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF",
	"llama2":      "TheBloke/Llama-2-7B-Chat-GGUF",
	"llama2-13b":  "TheBloke/Llama-2-13B-chat-GGUF",
	"mistral":     "TheBloke/Mistral-7B-Instruct-v0.2-GGUF",
	"mixtral":     "TheBloke/Mixtral-8x7B-Instruct-v0.1-GGUF",
	"phi2":        "TheBloke/phi-2-GGUF",
	"codellama":   "TheBloke/CodeLlama-7B-Instruct-GGUF",
	"zephyr":      "TheBloke/zephyr-7B-beta-GGUF",
	"openchat":    "TheBloke/openchat-3.5-0106-GGUF",
	"solar":       "TheBloke/SOLAR-10.7B-Instruct-v1.0-GGUF",
	"orca2":       "TheBloke/Orca-2-7B-GGUF",
	"stablelm":    "TheBloke/stablelm-zephyr-3b-GGUF",
	"deepseek":    "TheBloke/deepseek-coder-6.7B-instruct-GGUF",
	"neural-chat": "TheBloke/neural-chat-7B-v3-3-GGUF",
	"qwen":        "Qwen/Qwen1.5-7B-Chat-GGUF",
	"gemma":       "google/gemma-2b-it-GGUF",
}

// Entry represents a locally cached model.
type Entry struct {
	ID         string    `json:"id"`
	RepoID     string    `json:"repo_id"`
	Filename   string    `json:"filename"`
	FilePath   string    `json:"file_path"`
	SizeBytes  int64     `json:"size_bytes"`
	Downloaded time.Time `json:"downloaded"`
}

// Registry manages the local model cache.
type Registry struct {
	cfg     *config.Config
	dbPath  string
	entries []Entry
}

// NewRegistry initialises the registry and loads existing entries.
func NewRegistry(cfg *config.Config) *Registry {
	r := &Registry{
		cfg:    cfg,
		dbPath: filepath.Join(cfg.HomeDir, "models.json"),
	}
	r.load()
	return r
}

// ResolveAlias maps a short name to a full HuggingFace repo ID.
// If the name is already a repo path (contains '/'), it is returned as-is.
func ResolveAlias(name string) string {
	if repo, ok := Aliases[name]; ok {
		return repo
	}
	return name
}

// Find returns a cached entry by repo ID, or nil.
func (r *Registry) Find(repoID string) *Entry {
	for i := range r.entries {
		if r.entries[i].RepoID == repoID {
			return &r.entries[i]
		}
	}
	return nil
}

// Add registers a newly downloaded model.
func (r *Registry) Add(e Entry) error {
	// Replace if exists
	for i := range r.entries {
		if r.entries[i].RepoID == e.RepoID {
			r.entries[i] = e
			return r.save()
		}
	}
	r.entries = append(r.entries, e)
	return r.save()
}

// List returns all cached models.
func (r *Registry) List() []Entry {
	return r.entries
}

// Remove deletes a cached model by ID or repo ID.
func (r *Registry) Remove(id string) error {
	for i := range r.entries {
		if r.entries[i].ID == id || r.entries[i].RepoID == id {
			// Delete model file
			if r.entries[i].FilePath != "" {
				os.Remove(r.entries[i].FilePath)
			}
			// Delete model directory (best effort)
			dir := filepath.Dir(r.entries[i].FilePath)
			os.Remove(dir)

			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			return r.save()
		}
	}
	return fmt.Errorf("model %q not found", id)
}

func (r *Registry) load() {
	data, err := os.ReadFile(r.dbPath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &r.entries)
}

func (r *Registry) save() error {
	data, err := json.MarshalIndent(r.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.dbPath, data, 0644)
}
