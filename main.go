package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/llmgw/llmgw/internal/api"
	"github.com/llmgw/llmgw/internal/backend"
	"github.com/llmgw/llmgw/internal/config"
	"github.com/llmgw/llmgw/internal/downloader"
	"github.com/llmgw/llmgw/internal/huggingface"
	"github.com/llmgw/llmgw/internal/models"
	"github.com/llmgw/llmgw/internal/ui"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "search":
		cmdSearch(os.Args[2:])
	case "list":
		cmdList()
	case "remove":
		cmdRemove(os.Args[2:])
	case "aliases":
		cmdAliases()
	case "version":
		fmt.Printf("llmgw %s\n", config.Version)
	case "help", "--help", "-h":
		printUsage()
	default:
		ui.Error("Unknown command: %s", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// ──────────────────────────────────── run ────────────────────────────────────

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	port := fs.Int("port", config.DefaultPort, "API port")
	ctx := fs.Int("context", config.DefaultCtx, "Context window size")
	quant := fs.String("quant", "", "Preferred quantization (e.g. Q4_K_M)")
	verbose := fs.Bool("verbose", false, "Show backend output")
	token := fs.String("token", os.Getenv("HF_TOKEN"), "HuggingFace token")
	fs.Parse(args)

	if fs.NArg() < 1 {
		ui.Error("Usage: llmgw run <model> [flags]")
		ui.Detail("Example: llmgw run tinyllama")
		ui.Detail("Example: llmgw run TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF")
		os.Exit(1)
	}

	modelArg := fs.Arg(0)

	// Setup
	cfg := config.New()
	cfg.Port = *port
	cfg.CtxSize = *ctx
	cfg.Verbose = *verbose
	cfg.Quant = *quant

	if err := cfg.EnsureDirs(); err != nil {
		ui.Error("Failed to create directories: %v", err)
		os.Exit(1)
	}

	ui.Banner()

	// 1. Resolve model name
	repoID := models.ResolveAlias(modelArg)
	if repoID != modelArg {
		ui.Info("Resolved alias %q → %s", modelArg, repoID)
	}

	registry := models.NewRegistry(cfg)

	// 2. Check local cache
	var modelPath string
	if entry := registry.Find(repoID); entry != nil {
		if _, err := os.Stat(entry.FilePath); err == nil {
			ui.Success("Model found in cache: %s", entry.Filename)
			modelPath = entry.FilePath
		}
	}

	// 3. Download model if needed
	if modelPath == "" {
		ui.Step(1, 3, "Fetching model info from HuggingFace...")
		hf := huggingface.NewClient(*token)

		info, err := hf.GetModelInfo(repoID)
		if err != nil {
			ui.Error("Could not find model %q: %v", repoID, err)
			os.Exit(1)
		}

		ggufFiles := hf.FindGGUFFiles(info)
		if len(ggufFiles) == 0 {
			ui.Error("No GGUF files found in %s", repoID)
			ui.Detail("This repo may not contain quantized GGUF models.")
			ui.Detail("Try searching: llmgw search %s", modelArg)
			os.Exit(1)
		}

		selected := hf.SelectBestGGUF(ggufFiles, cfg.Quant)
		if selected == nil {
			ui.Error("Could not select a GGUF file")
			os.Exit(1)
		}

		ui.Info("Selected: %s", selected.Filename)
		if selected.Size > 0 {
			ui.Detail("Size: %s", ui.FormatBytes(selected.Size))
		}

		ui.Step(2, 3, "Downloading model...")
		dlURL := hf.DownloadURL(repoID, selected.Filename)
		modelDir := cfg.ModelDir(repoID)
		destPath := filepath.Join(modelDir, selected.Filename)

		if err := downloader.DownloadFile(dlURL, destPath, selected.Filename); err != nil {
			ui.Error("Download failed: %v", err)
			os.Exit(1)
		}

		// Register in cache
		registry.Add(models.Entry{
			ID:         repoID + "/" + selected.Filename,
			RepoID:     repoID,
			Filename:   selected.Filename,
			FilePath:   destPath,
			SizeBytes:  selected.Size,
			Downloaded: time.Now(),
		})

		ui.Success("Model downloaded")
		modelPath = destPath
	}

	// 4. Ensure backend
	ui.Step(3, 3, "Preparing inference backend...")
	mgr := backend.New(cfg)
	if err := mgr.EnsureBackend(); err != nil {
		ui.Error("Backend setup failed: %v", err)
		os.Exit(1)
	}

	// 5. Start backend
	ui.Info("Loading model into memory...")
	if err := mgr.Start(modelPath); err != nil {
		ui.Error("Failed to start backend: %v", err)
		os.Exit(1)
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println()
		ui.Info("Shutting down...")
		mgr.Stop()
		os.Exit(0)
	}()

	// 6. Wait for backend ready
	ui.Info("Waiting for model to load (this may take a moment)...")
	if err := mgr.WaitReady(5 * time.Minute); err != nil {
		ui.Error("Backend failed to start: %v", err)
		mgr.Stop()
		os.Exit(1)
	}

	// 7. Start API server
	ui.ServerReady(cfg.Port, repoID)

	srv := api.NewServer(cfg.Port, mgr.BackendURL(), repoID)
	if err := srv.ListenAndServe(); err != nil {
		ui.Error("Server error: %v", err)
		mgr.Stop()
		os.Exit(1)
	}
}

// ──────────────────────────────────── search ─────────────────────────────────

func cmdSearch(args []string) {
	if len(args) == 0 {
		ui.Error("Usage: llmgw search <query>")
		os.Exit(1)
	}

	query := strings.Join(args, " ")
	ui.Banner()
	ui.Info("Searching HuggingFace for %q ...", query)

	hf := huggingface.NewClient(os.Getenv("HF_TOKEN"))
	results, err := hf.Search(query)
	if err != nil {
		ui.Error("Search failed: %v", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		ui.Warn("No GGUF models found for %q", query)
		return
	}

	fmt.Println()
	fmt.Printf("  %-50s %10s %8s\n", "MODEL", "DOWNLOADS", "LIKES")
	fmt.Printf("  %s\n", strings.Repeat("─", 72))
	for _, r := range results {
		fmt.Printf("  %-50s %10d %8d\n", truncate(r.ID, 50), r.Downloads, r.Likes)
	}
	fmt.Println()
	ui.Detail("Run with: llmgw run <model-id>")
}

// ──────────────────────────────────── list ───────────────────────────────────

func cmdList() {
	cfg := config.New()
	registry := models.NewRegistry(cfg)
	entries := registry.List()

	ui.Banner()

	if len(entries) == 0 {
		ui.Info("No models cached. Download one with: llmgw run <model>")
		return
	}

	fmt.Printf("  %-45s %-30s %10s\n", "REPO", "FILE", "SIZE")
	fmt.Printf("  %s\n", strings.Repeat("─", 88))
	for _, e := range entries {
		fmt.Printf("  %-45s %-30s %10s\n",
			truncate(e.RepoID, 45),
			truncate(e.Filename, 30),
			ui.FormatBytes(e.SizeBytes))
	}
	fmt.Println()
}

// ──────────────────────────────────── remove ─────────────────────────────────

func cmdRemove(args []string) {
	if len(args) == 0 {
		ui.Error("Usage: llmgw remove <model-id>")
		os.Exit(1)
	}

	cfg := config.New()
	registry := models.NewRegistry(cfg)

	target := models.ResolveAlias(args[0])
	if err := registry.Remove(target); err != nil {
		ui.Error("%v", err)
		os.Exit(1)
	}
	ui.Success("Removed %s", target)
}

// ──────────────────────────────────── aliases ────────────────────────────────

func cmdAliases() {
	ui.Banner()
	fmt.Printf("  %-15s %s\n", "ALIAS", "HUGGINGFACE REPO")
	fmt.Printf("  %s\n", strings.Repeat("─", 70))
	for alias, repo := range models.Aliases {
		fmt.Printf("  %-15s %s\n", alias, repo)
	}
	fmt.Println()
	ui.Detail("Use an alias: llmgw run tinyllama")
}

// ──────────────────────────────────── help ───────────────────────────────────

func printUsage() {
	ui.Banner()
	fmt.Println("  " + ui.Bold + "USAGE" + ui.Reset)
	fmt.Println("    llmgw <command> [arguments]")
	fmt.Println()
	fmt.Println("  " + ui.Bold + "COMMANDS" + ui.Reset)
	fmt.Println("    run <model>       Download & serve a model with OpenAI-compatible API")
	fmt.Println("    search <query>    Search HuggingFace for GGUF models")
	fmt.Println("    list              List locally cached models")
	fmt.Println("    remove <model>    Remove a cached model")
	fmt.Println("    aliases           Show built-in model aliases")
	fmt.Println("    version           Print version")
	fmt.Println()
	fmt.Println("  " + ui.Bold + "FLAGS (for run)" + ui.Reset)
	fmt.Println("    -port      int    API port          (default: 8080)")
	fmt.Println("    -context   int    Context window    (default: 4096)")
	fmt.Println("    -quant     string Preferred quant   (e.g. Q4_K_M)")
	fmt.Println("    -verbose          Show backend logs")
	fmt.Println("    -token     string HuggingFace token (or HF_TOKEN env)")
	fmt.Println()
	fmt.Println("  " + ui.Bold + "EXAMPLES" + ui.Reset)
	fmt.Println("    llmgw run tinyllama")
	fmt.Println("    llmgw run TheBloke/Mistral-7B-Instruct-v0.2-GGUF -port 9000")
	fmt.Println("    llmgw search \"code llama\"")
	fmt.Println("    llmgw list")
	fmt.Println()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
