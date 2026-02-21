package ui

import (
	"fmt"
	"os"
	"strings"
)

// ANSI color codes
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Cyan    = "\033[36m"
)

// Banner prints the startup banner.
func Banner() {
	fmt.Println()
	fmt.Printf("%s%s ⚡ LLM Gateway v1.0.0 %s\n", Bold, Cyan, Reset)
	fmt.Printf("%s    Zero-Config Local LLM Server%s\n", Dim, Reset)
	fmt.Printf("%s%s%s\n", Dim, strings.Repeat("─", 42), Reset)
	fmt.Println()
}

// Info prints an informational message.
func Info(format string, args ...interface{}) {
	fmt.Printf("%s%s⬥%s %s\n", Bold, Blue, Reset, fmt.Sprintf(format, args...))
}

// Success prints a success message.
func Success(format string, args ...interface{}) {
	fmt.Printf("%s%s✓%s %s\n", Bold, Green, Reset, fmt.Sprintf(format, args...))
}

// Warn prints a warning message.
func Warn(format string, args ...interface{}) {
	fmt.Printf("%s%s⚠%s %s\n", Bold, Yellow, Reset, fmt.Sprintf(format, args...))
}

// Error prints an error message.
func Error(format string, args ...interface{}) {
	fmt.Printf("%s%s✗%s %s\n", Bold, Red, Reset, fmt.Sprintf(format, args...))
}

// Step prints a numbered step.
func Step(n, total int, format string, args ...interface{}) {
	fmt.Printf("%s[%d/%d]%s %s\n", Cyan, n, total, Reset, fmt.Sprintf(format, args...))
}

// Detail prints an indented detail line.
func Detail(format string, args ...interface{}) {
	fmt.Printf("      %s%s%s\n", Dim, fmt.Sprintf(format, args...), Reset)
}

// ServerReady prints the ready banner with endpoint info.
func ServerReady(port int, model string) {
	fmt.Println()
	fmt.Printf("%s%s╔══════════════════════════════════════════════╗%s\n", Bold, Green, Reset)
	fmt.Printf("%s%s║         🚀 LLM Gateway is READY!             ║%s\n", Bold, Green, Reset)
	fmt.Printf("%s%s╚══════════════════════════════════════════════╝%s\n", Bold, Green, Reset)
	fmt.Println()
	fmt.Printf("  %sModel:%s  %s\n", Bold, Reset, model)
	fmt.Printf("  %sAPI:%s    http://localhost:%d/v1\n", Bold, Reset, port)
	fmt.Println()
	fmt.Printf("  %sEndpoints:%s\n", Bold, Reset)
	fmt.Printf("    %sPOST%s /v1/chat/completions\n", Cyan, Reset)
	fmt.Printf("    %sPOST%s /v1/completions\n", Cyan, Reset)
	fmt.Printf("    %sGET %s /v1/models\n", Cyan, Reset)
	fmt.Printf("    %sGET %s /health\n", Cyan, Reset)
	fmt.Println()
	fmt.Printf("  %sQuick test:%s\n", Bold, Reset)
	fmt.Printf("    curl http://localhost:%d/v1/chat/completions \\\n", port)
	fmt.Printf("      -H \"Content-Type: application/json\" \\\n")
	fmt.Printf("      -d '{\"model\":\"%s\",\"messages\":[{\"role\":\"user\",\"content\":\"Hi\"}]}'\n", model)
	fmt.Println()
	fmt.Printf("  %sPress Ctrl+C to stop the server%s\n", Dim, Reset)
	fmt.Println()
}

// ProgressBar is a simple terminal progress bar.
type ProgressBar struct {
	total   int64
	current int64
	width   int
	label   string
}

// NewProgressBar creates a progress bar.
func NewProgressBar(total int64, label string) *ProgressBar {
	return &ProgressBar{total: total, width: 40, label: label}
}

// Update redraws the progress bar.
func (p *ProgressBar) Update(current int64) {
	p.current = current
	if p.total <= 0 {
		fmt.Fprintf(os.Stderr, "\r  %s %s", p.label, FormatBytes(current))
		return
	}
	pct := float64(current) / float64(p.total) * 100
	filled := int(float64(p.width) * float64(current) / float64(p.total))
	if filled > p.width {
		filled = p.width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", p.width-filled)
	sz := FormatBytes(current) + "/" + FormatBytes(p.total)
	fmt.Fprintf(os.Stderr, "\r  %s %s[%s%s%s]%s %.1f%% %s  ", p.label, Reset, Cyan, bar, Reset, Reset, pct, sz)
}

// Finish completes the progress bar.
func (p *ProgressBar) Finish() {
	if p.total > 0 {
		p.Update(p.total)
	}
	fmt.Fprintln(os.Stderr)
}

// FormatBytes formats bytes into human-readable form.
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
