package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var (
	workPruneDryRun        bool
	workPruneIncludeFailed bool
	workPruneOlderThan     time.Duration
)

type workPruneResult struct {
	RemovedJobs    int
	RemovedBytes   int64
	KeptFailedJobs int
	ProtectedJobs  int
	MatchedJobs    int
}

var workCmd = &cobra.Command{
	Use:   "work",
	Short: "Manage staged extract work directories",
}

var workPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old staged extract work directories",
	RunE:  runWorkPrune,
}

func init() {
	workPruneCmd.Flags().BoolVar(&workPruneDryRun, "dry-run", false, "Show which jobs would be removed without deleting them")
	workPruneCmd.Flags().BoolVar(&workPruneIncludeFailed, "include-failed", false, "Also remove failed jobs older than the threshold")
	workPruneCmd.Flags().DurationVar(&workPruneOlderThan, "older-than", 168*time.Hour, "Only remove jobs older than this duration")
	workCmd.AddCommand(workPruneCmd)
}

func runWorkPrune(cmd *cobra.Command, args []string) error {
	workRoot, err := extractWorkRoot()
	if err != nil {
		return err
	}

	result, err := pruneExtractWorkdirs(workRoot, time.Now(), workPruneOlderThan, workPruneIncludeFailed, workPruneDryRun)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(),
		"removed_jobs=%d removed_bytes=%d kept_failed_jobs=%d protected_jobs=%d matched_jobs=%d dry_run=%t\n",
		result.RemovedJobs,
		result.RemovedBytes,
		result.KeptFailedJobs,
		result.ProtectedJobs,
		result.MatchedJobs,
		workPruneDryRun,
	); err != nil {
		return err
	}
	return nil
}

func pruneExtractWorkdirs(workRoot string, now time.Time, olderThan time.Duration, includeFailed, dryRun bool) (workPruneResult, error) {
	result := workPruneResult{}
	entries, err := os.ReadDir(workRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, fmt.Errorf("read work root: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jobDir := filepath.Join(workRoot, entry.Name())
		manifest, err := loadExtractManifest(jobDir)
		if err != nil {
			continue
		}

		status := manifest.Status
		if status == "failed" && !includeFailed {
			result.KeptFailedJobs++
			continue
		}
		if status != "completed" && status != "failed" {
			result.ProtectedJobs++
			continue
		}

		manifestPath := filepath.Join(jobDir, "manifest.json")
		info, err := os.Stat(manifestPath)
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) < olderThan {
			continue
		}

		result.MatchedJobs++
		size, err := dirSize(jobDir)
		if err != nil {
			return result, err
		}
		if dryRun {
			result.RemovedJobs++
			result.RemovedBytes += size
			continue
		}
		if err := os.RemoveAll(jobDir); err != nil {
			return result, fmt.Errorf("remove work dir %s: %w", jobDir, err)
		}
		result.RemovedJobs++
		result.RemovedBytes += size
	}

	return result, nil
}

func dirSize(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk %s: %w", path, err)
	}
	return total, nil
}
