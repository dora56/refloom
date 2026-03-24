package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var updateCheckCmd = &cobra.Command{
	Use:   "update-check",
	Short: "Check for newer versions on GitHub",
	RunE:  runUpdateCheck,
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

func runUpdateCheck(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	latest, err := fetchLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("check update: %w", err)
	}

	current := Version
	latestTag := strings.TrimPrefix(latest.TagName, "v")
	currentClean := strings.TrimPrefix(current, "v")

	if latestTag == currentClean || current == "dev" {
		fmt.Printf("You are up to date (current: %s)\n", current)
		return nil
	}

	fmt.Printf("New version available: %s (current: %s)\n", latest.TagName, current)
	fmt.Printf("Download: %s\n", latest.HTMLURL)
	return nil
}

func fetchLatestRelease(ctx context.Context) (*githubRelease, error) {
	url := "https://api.github.com/repos/dora56/refloom/releases/latest"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases found (repository may not exist or be private)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &release, nil
}
