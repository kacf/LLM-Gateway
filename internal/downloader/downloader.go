package downloader

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/llmgw/llmgw/internal/ui"
)

// DownloadFile downloads a URL to destPath, showing a progress bar.
// If the file already exists and is non-empty, it skips the download.
func DownloadFile(url, destPath, label string) error {
	if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	tmpPath := destPath + ".download"

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("requesting %s: %w", label, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s failed: HTTP %d", label, resp.StatusCode)
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	var writeErr error
	if resp.ContentLength > 0 {
		bar := ui.NewProgressBar(resp.ContentLength, label)
		pr := &progressReader{reader: resp.Body, bar: bar}
		_, writeErr = io.Copy(out, pr)
		bar.Finish()
	} else {
		_, writeErr = io.Copy(out, resp.Body)
	}

	out.Close()
	if writeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("writing %s: %w", label, writeErr)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("finalizing %s: %w", label, err)
	}
	return nil
}

type progressReader struct {
	reader  io.Reader
	bar     *ui.ProgressBar
	current int64
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.current += int64(n)
	pr.bar.Update(pr.current)
	return n, err
}
