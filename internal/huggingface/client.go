package huggingface

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

const (
	baseURL = "https://huggingface.co"
	apiURL  = "https://huggingface.co/api"
)

// Client communicates with the HuggingFace Hub API.
type Client struct {
	http  *http.Client
	token string
}

// ModelInfo is the response from /api/models/{id}.
type ModelInfo struct {
	ID       string     `json:"id"`
	Author   string     `json:"author"`
	Tags     []string   `json:"tags"`
	Siblings []FileInfo `json:"siblings"`
}

// FileInfo describes one file in a model repo.
type FileInfo struct {
	Filename string `json:"rfilename"`
	Size     int64  `json:"size"`
}

// SearchResult is one item from a model search.
type SearchResult struct {
	ID        string   `json:"id"`
	Downloads int      `json:"downloads"`
	Likes     int      `json:"likes"`
	Tags      []string `json:"tags"`
}

// NewClient creates a HuggingFace client. token may be empty.
func NewClient(token string) *Client {
	return &Client{http: &http.Client{}, token: token}
}

// GetModelInfo fetches metadata for the given repo (e.g. "TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF").
func (c *Client) GetModelInfo(repoID string) (*ModelInfo, error) {
	u := fmt.Sprintf("%s/models/%s", apiURL, repoID)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching model info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HuggingFace API returned %d: %s", resp.StatusCode, string(body))
	}

	var info ModelInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding model info: %w", err)
	}
	return &info, nil
}

// FindGGUFFiles returns all .gguf files in the model repo.
func (c *Client) FindGGUFFiles(info *ModelInfo) []FileInfo {
	var out []FileInfo
	for _, f := range info.Siblings {
		if strings.HasSuffix(strings.ToLower(f.Filename), ".gguf") {
			out = append(out, f)
		}
	}
	return out
}

// SelectBestGGUF picks the best quantization from available GGUF files.
// Preferred order: Q4_K_M > Q4_K_S > Q5_K_M > Q5_K_S > Q4_0 > Q8_0 > smallest.
func (c *Client) SelectBestGGUF(files []FileInfo, preferred string) *FileInfo {
	if len(files) == 0 {
		return nil
	}

	// If user specified a preference, try to match it
	if preferred != "" {
		for i := range files {
			if strings.Contains(strings.ToLower(files[i].Filename), strings.ToLower(preferred)) {
				return &files[i]
			}
		}
	}

	// Priority-based selection
	priorities := []string{"Q4_K_M", "Q4_K_S", "q4_k_m", "q4_k_s", "Q5_K_M", "Q5_K_S", "Q4_0", "Q8_0"}
	for _, q := range priorities {
		for i := range files {
			if strings.Contains(files[i].Filename, q) {
				return &files[i]
			}
		}
	}

	// Fallback: pick the smallest file
	sort.Slice(files, func(i, j int) bool { return files[i].Size < files[j].Size })
	return &files[0]
}

// DownloadURL returns the direct download URL for a file in a repo.
func (c *Client) DownloadURL(repoID, filename string) string {
	return fmt.Sprintf("%s/%s/resolve/main/%s", baseURL, repoID, filename)
}

// Search queries HuggingFace for GGUF models matching the query.
func (c *Client) Search(query string) ([]SearchResult, error) {
	params := url.Values{}
	params.Set("search", query)
	params.Set("filter", "gguf")
	params.Set("sort", "downloads")
	params.Set("direction", "-1")
	params.Set("limit", "20")

	u := fmt.Sprintf("%s/models?%s", apiURL, params.Encode())
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searching models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned HTTP %d", resp.StatusCode)
	}

	var results []SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("decoding search results: %w", err)
	}
	return results, nil
}
