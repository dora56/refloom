package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dora56/refloom/internal/db"
	"github.com/dora56/refloom/internal/embedding"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and dependencies",
	RunE:  runDoctor,
}

// CheckResult holds the result of a single health check.
type CheckResult struct {
	Name   string
	Status string // ok, warn, fail
	Detail string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	var checks []CheckResult

	checks = append(checks, checkDB())
	checks = append(checks, checkOllama())
	checks = append(checks, checkPythonWorker())
	checks = append(checks, checkDisk())
	checks = append(checks, checkAutoExtract())

	hasFailure := false
	for _, c := range checks {
		icon := "✓"
		switch c.Status {
		case "warn":
			icon = "!"
		case "fail":
			icon = "✗"
			hasFailure = true
		}
		fmt.Printf("[%s] %s: %s\n", icon, c.Name, c.Detail)
	}

	if hasFailure {
		return fmt.Errorf("some checks failed")
	}
	return nil
}

func checkDB() CheckResult {
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return CheckResult{Name: "Database", Status: "fail", Detail: fmt.Sprintf("cannot open: %v", err)}
	}
	defer database.Close() //nolint:errcheck

	// Run integrity check
	var result string
	if err := database.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return CheckResult{Name: "Database", Status: "fail", Detail: fmt.Sprintf("integrity check error: %v", err)}
	}
	if result != "ok" {
		return CheckResult{Name: "Database", Status: "fail", Detail: fmt.Sprintf("integrity check: %s", result)}
	}

	// Count books
	books, err := database.ListBooks()
	if err != nil {
		return CheckResult{Name: "Database", Status: "warn", Detail: fmt.Sprintf("ok (cannot count books: %v)", err)}
	}

	return CheckResult{Name: "Database", Status: "ok", Detail: fmt.Sprintf("ok (%d books)", len(books))}
}

func checkOllama() CheckResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := embedding.NewClient(cfg.OllamaURL, cfg.OllamaEmbedModel)
	if err := client.CheckHealth(ctx); err != nil {
		return CheckResult{Name: "Ollama", Status: "fail", Detail: fmt.Sprintf("%v", err)}
	}

	return CheckResult{Name: "Ollama", Status: "ok", Detail: fmt.Sprintf("%s @ %s", cfg.OllamaEmbedModel, cfg.OllamaURL)}
}

func checkPythonWorker() CheckResult {
	workerDir, pythonPath := findWorkerPaths()

	if _, err := os.Stat(pythonPath); err != nil {
		return CheckResult{Name: "Python Worker", Status: "fail", Detail: fmt.Sprintf("python not found: %s", pythonPath)}
	}

	return CheckResult{Name: "Python Worker", Status: "ok", Detail: fmt.Sprintf("%s (dir: %s)", pythonPath, workerDir)}
}

func checkDisk() CheckResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return CheckResult{Name: "Disk", Status: "warn", Detail: "cannot determine home directory"}
	}

	dbPath := cfg.DBPath
	if dbPath == "" {
		dbPath = filepath.Join(home, ".refloom", "refloom.db")
	}
	stat, err := os.Stat(dbPath)
	if err != nil {
		return CheckResult{Name: "Disk", Status: "ok", Detail: "database file not yet created"}
	}

	sizeMB := float64(stat.Size()) / 1024 / 1024

	// OCR cache size
	cacheDir := filepath.Join(home, ".refloom", "cache", "ocr")
	cacheFiles, cacheSizeMB := countDirSize(cacheDir)
	if cacheFiles > 0 {
		return CheckResult{Name: "Disk", Status: "ok", Detail: fmt.Sprintf("database: %.1f MB, OCR cache: %.1f MB (%d files)", sizeMB, cacheSizeMB, cacheFiles)}
	}
	return CheckResult{Name: "Disk", Status: "ok", Detail: fmt.Sprintf("database: %.1f MB, OCR cache: none", sizeMB)}
}

func checkAutoExtract() CheckResult {
	host := observeHostExtractCapacity()
	status := "ok"
	if autoExtractObservationDegraded(host) {
		status = "warn"
	}
	return CheckResult{
		Name:   "Auto Extract",
		Status: status,
		Detail: doctorAutoExtractDetail(host, cfg.ExtractAutoMaxWorkers),
	}
}

func autoExtractObservationDegraded(host hostExtractObservation) bool {
	return host.PerfCores <= 0 || host.TotalMemBytes == 0 || host.FreeMemBytes == 0
}

func countDirSize(dir string) (files int, sizeMB float64) {
	var totalBytes int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		files++
		totalBytes += info.Size()
		return nil
	})
	sizeMB = float64(totalBytes) / 1024 / 1024
	return files, sizeMB
}
