package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dora56/refloom/internal/config"
	"github.com/dora56/refloom/internal/extraction"
)

// mockExtractor implements extraction.Extractor for testing.
type mockExtractor struct {
	probeResp      *extraction.ProbeResponse
	extractPagesFn func(ctx context.Context, req extraction.ExtractPagesRequest) (*extraction.ExtractPagesResponse, error)
	chunkResp      *extraction.ChunkResponse
}

func (m *mockExtractor) Probe(_ context.Context, _, _ string) (*extraction.ProbeResponse, error) {
	return m.probeResp, nil
}

func (m *mockExtractor) ExtractPages(ctx context.Context, req extraction.ExtractPagesRequest) (*extraction.ExtractPagesResponse, error) {
	if m.extractPagesFn != nil {
		return m.extractPagesFn(ctx, req)
	}
	return &extraction.ExtractPagesResponse{PagesWritten: req.PageEnd - req.PageStart + 1}, nil
}

func (m *mockExtractor) Chunk(_ context.Context, _ extraction.ChunkRequest) (*extraction.ChunkResponse, error) {
	return m.chunkResp, nil
}

func TestBuildPageBatchRanges(t *testing.T) {
	t.Parallel()

	got := buildPageBatchRanges(70, 16)
	want := []pageBatchRange{
		{PageStart: 1, PageEnd: 16},
		{PageStart: 17, PageEnd: 32},
		{PageStart: 33, PageEnd: 48},
		{PageStart: 49, PageEnd: 64},
		{PageStart: 65, PageEnd: 70},
	}

	if len(got) != len(want) {
		t.Fatalf("len(ranges) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("range[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestResolvedExtractBatchSize(t *testing.T) {
	t.Parallel()

	if got := resolvedExtractBatchSize("ocr-heavy", 0); got != defaultOCRBatchSize {
		t.Fatalf("ocr-heavy batch size = %d, want %d", got, defaultOCRBatchSize)
	}
	if got := resolvedExtractBatchSize("text", 0); got != defaultTextBatchSize {
		t.Fatalf("text batch size = %d, want %d", got, defaultTextBatchSize)
	}
	if got := resolvedExtractBatchSize("text", 24); got != 24 {
		t.Fatalf("configured batch size = %d, want 24", got)
	}
}

func TestMergePageBatches(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "000001-000002.jsonl")
	second := filepath.Join(dir, "000003-000004.jsonl")
	if err := os.WriteFile(first, []byte("{\"page_num\":1}\n{\"page_num\":2}\n"), 0o600); err != nil {
		t.Fatalf("write first batch: %v", err)
	}
	if err := os.WriteFile(second, []byte("{\"page_num\":3}\n"), 0o600); err != nil {
		t.Fatalf("write second batch: %v", err)
	}

	output := filepath.Join(dir, "pages.all.jsonl")
	err := mergePageBatches(output, []extractCompletedBatch{
		{PageStart: 3, PageEnd: 4, OutputPath: second},
		{PageStart: 1, PageEnd: 2, OutputPath: first},
	})
	if err != nil {
		t.Fatalf("mergePageBatches: %v", err)
	}

	data, err := os.ReadFile(output) //nolint:gosec
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	want := "{\"page_num\":1}\n{\"page_num\":2}\n{\"page_num\":3}\n"
	if string(data) != want {
		t.Fatalf("merged data = %q, want %q", string(data), want)
	}

	// Verify restrictive file permissions
	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("permissions = %o, want 0600", perm)
	}
}

func TestResolveOCRPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode string
		want string
	}{
		{"ocr-heavy", "accurate-only"},
		{"OCR-HEAVY", "accurate-only"},
		{"text", "auto"},
		{"", "auto"},
		{"mixed", "auto"},
	}
	for _, tc := range tests {
		if got := resolveOCRPolicy(tc.mode); got != tc.want {
			t.Errorf("resolveOCRPolicy(%q) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestWriteJSONFileAtomic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	data := map[string]string{"key": "value"}
	if err := writeJSONFile(path, data); err != nil {
		t.Fatalf("writeJSONFile: %v", err)
	}

	// File should exist with correct content
	content, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(content), `"key": "value"`) {
		t.Fatalf("content = %q, want key:value", string(content))
	}

	// No .tmp file should remain
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf(".tmp file should not exist after successful write")
	}

	// Permissions should be 0600
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("permissions = %o, want 0600", perm)
	}
}

func TestPrepareExtractJobResetsCompletedManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	jobDir := filepath.Join(home, ".refloom", "work", "job-1")
	if err := os.MkdirAll(filepath.Join(jobDir, "pages"), 0o750); err != nil {
		t.Fatalf("mkdir pages: %v", err)
	}
	manifest := extractJobManifest{
		JobID:      "job-1",
		SourcePath: "/tmp/book.pdf",
		Format:     "pdf",
		Status:     "completed",
		Completed: []extractCompletedBatch{
			{PageStart: 1, PageEnd: 16, OutputPath: filepath.Join(jobDir, "pages", "000001-000016.jsonl")},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "manifest.json"), data, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "pages", "stale.jsonl"), []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale page: %v", err)
	}

	gotDir, gotManifest, resumed, err := prepareExtractJob("/tmp/book.pdf", "pdf", "job-1")
	if err != nil {
		t.Fatalf("prepareExtractJob: %v", err)
	}
	if resumed {
		t.Fatalf("resumed = true, want false for completed manifest")
	}
	if gotDir != jobDir {
		t.Fatalf("jobDir = %q, want %q", gotDir, jobDir)
	}
	if gotManifest.Status != "created" {
		t.Fatalf("status = %q, want created", gotManifest.Status)
	}
	if len(gotManifest.Completed) != 0 {
		t.Fatalf("completed batches = %d, want 0", len(gotManifest.Completed))
	}
	if _, err := os.Stat(filepath.Join(jobDir, "pages", "stale.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("stale page file should be removed, stat err = %v", err)
	}
}

func TestApplyExtractBatchResultTracksOnlySumMetrics(t *testing.T) {
	t.Parallel()

	jobDir := t.TempDir()
	manifest := &extractJobManifest{}
	result := extractPagesResult{
		batch: pageBatchRange{PageStart: 1, PageEnd: 16},
		completed: &extractCompletedBatch{
			PageStart: 1,
			PageEnd:   16,
			BatchMS:   1250,
			Stats: extraction.Stats{
				OCRMS: 990,
			},
		},
	}

	if err := applyExtractBatchResult(jobDir, manifest, result); err != nil {
		t.Fatalf("applyExtractBatchResult: %v", err)
	}

	if manifest.PageExtractMS != 0 {
		t.Fatalf("PageExtractMS = %d, want 0 before wall-clock finalization", manifest.PageExtractMS)
	}
	if manifest.PageExtractSumMS != 1250 {
		t.Fatalf("PageExtractSumMS = %d, want 1250", manifest.PageExtractSumMS)
	}
	if manifest.OCRMS != 990 {
		t.Fatalf("OCRMS = %d, want 990", manifest.OCRMS)
	}
}

func TestFinalizePageExtractMetricsUsesWallClock(t *testing.T) {
	t.Parallel()

	manifest := &extractJobManifest{
		ProbeMS:          200,
		PageExtractSumMS: 3200,
		ChunkMS:          40,
	}

	finalizePageExtractMetrics(manifest, 1500*time.Millisecond)

	if manifest.PageExtractMS != 1500 {
		t.Fatalf("PageExtractMS = %d, want 1500", manifest.PageExtractMS)
	}
	if manifest.PageExtractSumMS != 3200 {
		t.Fatalf("PageExtractSumMS = %d, want 3200", manifest.PageExtractSumMS)
	}
}

func TestResolveExtractWorkerPlanFixedMode(t *testing.T) {
	t.Parallel()

	plan := resolveExtractWorkerPlan(
		"pdf",
		"ocr-heavy",
		config.ExtractBatchWorkersSetting{Fixed: 4},
		8,
		6,
		hostExtractObservation{},
		0,
	)

	if plan.Mode != "fixed" {
		t.Fatalf("Mode = %q, want fixed", plan.Mode)
	}
	if plan.Requested != "4" {
		t.Fatalf("Requested = %q, want 4", plan.Requested)
	}
	if plan.Used != 4 {
		t.Fatalf("Used = %d, want 4", plan.Used)
	}
	if plan.Reason != "fixed workers requested" {
		t.Fatalf("Reason = %q, want fixed workers requested", plan.Reason)
	}
}

func TestResolveExtractWorkerPlanFixedNonOCRAlwaysOne(t *testing.T) {
	t.Parallel()

	plan := resolveExtractWorkerPlan(
		"pdf",
		"text",
		config.ExtractBatchWorkersSetting{Fixed: 4},
		8,
		6,
		hostExtractObservation{},
		0,
	)

	if plan.Used != 1 {
		t.Fatalf("Used = %d, want 1", plan.Used)
	}
	if plan.Reason != "parallel workers disabled for non-OCR-heavy PDF" {
		t.Fatalf("Reason = %q, want non-OCR fixed reason", plan.Reason)
	}
}

func TestResolveExtractWorkerPlanAutoNonOCRAlwaysOne(t *testing.T) {
	t.Parallel()

	plan := resolveExtractWorkerPlan(
		"epub",
		"text",
		config.DefaultExtractBatchWorkersSetting(),
		8,
		8,
		hostExtractObservation{PerfCores: 8},
		9000,
	)

	if plan.Mode != "auto" {
		t.Fatalf("Mode = %q, want auto", plan.Mode)
	}
	if plan.Used != 1 {
		t.Fatalf("Used = %d, want 1", plan.Used)
	}
	if plan.AutoTier != "none" {
		t.Fatalf("AutoTier = %q, want none", plan.AutoTier)
	}
	if plan.EffectiveAutoCap != 1 {
		t.Fatalf("EffectiveAutoCap = %d, want 1", plan.EffectiveAutoCap)
	}
	if !slices.Equal(plan.AutoCandidates, []int{1}) {
		t.Fatalf("AutoCandidates = %v, want [1]", plan.AutoCandidates)
	}
	if plan.Reason != "tier=none selected=1 reason=non-ocr-heavy" {
		t.Fatalf("Reason = %q, want non-OCR-heavy reason", plan.Reason)
	}
}

func TestResolveExtractWorkerPlanAutoRespectsConfiguredCap(t *testing.T) {
	t.Parallel()

	plan := resolveExtractWorkerPlan(
		"pdf",
		"ocr-heavy",
		config.DefaultExtractBatchWorkersSetting(),
		4,
		8,
		hostExtractObservation{
			PerfCores:     6,
			PhysicalCores: 8,
			TotalMemBytes: 16 << 30,
			FreeMemBytes:  8 << 30,
		},
		12000,
	)

	if plan.Used != 4 {
		t.Fatalf("Used = %d, want 4", plan.Used)
	}
	if plan.AutoTier != "pro" {
		t.Fatalf("AutoTier = %q, want pro", plan.AutoTier)
	}
	if plan.ConfiguredAutoCap != 4 {
		t.Fatalf("ConfiguredAutoCap = %d, want 4", plan.ConfiguredAutoCap)
	}
	if plan.EffectiveAutoCap != 4 {
		t.Fatalf("EffectiveAutoCap = %d, want 4", plan.EffectiveAutoCap)
	}
	if !slices.Equal(plan.AutoCandidates, []int{1, 2, 4}) {
		t.Fatalf("AutoCandidates = %v, want [1 2 4]", plan.AutoCandidates)
	}
	if !strings.Contains(plan.Reason, "tier=pro") || !strings.Contains(plan.Reason, "configured_cap=4") || !strings.Contains(plan.Reason, "effective_cap=4") || !strings.Contains(plan.Reason, "selected=4") {
		t.Fatalf("Reason = %q, want configured cap selection reason", plan.Reason)
	}
}

func TestResolveExtractWorkerPlanAutoCapsLowTotalMemory(t *testing.T) {
	t.Parallel()

	plan := resolveExtractWorkerPlan(
		"pdf",
		"ocr-heavy",
		config.DefaultExtractBatchWorkersSetting(),
		8,
		8,
		hostExtractObservation{
			PerfCores:     8,
			PhysicalCores: 10,
			TotalMemBytes: 4 << 30,
			FreeMemBytes:  3 << 30,
		},
		30000,
	)

	if plan.Used != 2 {
		t.Fatalf("Used = %d, want 2", plan.Used)
	}
	if plan.EffectiveAutoCap != 2 {
		t.Fatalf("EffectiveAutoCap = %d, want 2", plan.EffectiveAutoCap)
	}
	if !slices.Equal(plan.AutoCandidates, []int{1, 2}) {
		t.Fatalf("AutoCandidates = %v, want [1 2]", plan.AutoCandidates)
	}
	if !strings.Contains(plan.Reason, "selected=2") || !strings.Contains(plan.Reason, "memory_adjusted=total_mem_lt_8gib") {
		t.Fatalf("Reason = %q, want low total memory adjustment", plan.Reason)
	}
}

func TestResolveExtractWorkerPlanAutoDownshiftsOnLowFreeMemory(t *testing.T) {
	t.Parallel()

	plan := resolveExtractWorkerPlan(
		"pdf",
		"ocr-heavy",
		config.DefaultExtractBatchWorkersSetting(),
		8,
		8,
		hostExtractObservation{
			PerfCores:     8,
			PhysicalCores: 10,
			TotalMemBytes: 16 << 30,
			FreeMemBytes:  768 << 20,
		},
		30000,
	)

	if plan.Used != 6 {
		t.Fatalf("Used = %d, want 6", plan.Used)
	}
	if !strings.Contains(plan.Reason, "selected=6") || !strings.Contains(plan.Reason, "memory_adjusted=free_mem_lt_1gib") {
		t.Fatalf("Reason = %q, want low free memory downshift", plan.Reason)
	}
}

func TestResolveExtractWorkerPlanAutoCriticalFreeMemoryForcesOne(t *testing.T) {
	t.Parallel()

	plan := resolveExtractWorkerPlan(
		"pdf",
		"ocr-heavy",
		config.DefaultExtractBatchWorkersSetting(),
		8,
		8,
		hostExtractObservation{
			PerfCores:     8,
			PhysicalCores: 10,
			TotalMemBytes: 16 << 30,
			FreeMemBytes:  128 << 20,
		},
		30000,
	)

	if plan.Used != 1 {
		t.Fatalf("Used = %d, want 1", plan.Used)
	}
	if !strings.Contains(plan.Reason, "selected=1") || !strings.Contains(plan.Reason, "memory_adjusted=free_mem_lt_256mib") {
		t.Fatalf("Reason = %q, want critical free memory reason", plan.Reason)
	}
}

func TestResolveExtractWorkerPlanAutoUsesWarmupThresholds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		warmupMS  int64
		want      int
		wantCap   int
		wantCause string
	}{
		{name: "fast batches stay sequential", warmupMS: 1000, want: 1, wantCap: 8, wantCause: "avg_batch_ms=1000"},
		{name: "medium batches use two workers", warmupMS: 3000, want: 2, wantCap: 8, wantCause: "avg_batch_ms=3000"},
		{name: "slow batches use four workers", warmupMS: 7000, want: 4, wantCap: 8, wantCause: "avg_batch_ms=7000"},
		{name: "very slow batches use six workers", warmupMS: 15000, want: 6, wantCap: 8, wantCause: "avg_batch_ms=15000"},
		{name: "extremely slow batches use eight workers", warmupMS: 25000, want: 8, wantCap: 8, wantCause: "avg_batch_ms=25000"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			plan := resolveExtractWorkerPlan(
				"pdf",
				"ocr-heavy",
				config.DefaultExtractBatchWorkersSetting(),
				8,
				8,
				hostExtractObservation{
					PerfCores:     8,
					PhysicalCores: 10,
					TotalMemBytes: 16 << 30,
					FreeMemBytes:  8 << 30,
				},
				tc.warmupMS,
			)

			if plan.Used != tc.want {
				t.Fatalf("Used = %d, want %d", plan.Used, tc.want)
			}
			if plan.EffectiveAutoCap != tc.wantCap {
				t.Fatalf("EffectiveAutoCap = %d, want %d", plan.EffectiveAutoCap, tc.wantCap)
			}
			if !strings.Contains(plan.Reason, tc.wantCause) || !strings.Contains(plan.Reason, fmt.Sprintf("selected=%d", tc.want)) {
				t.Fatalf("Reason = %q, want cause containing %q and selected=%d", plan.Reason, tc.wantCause, tc.want)
			}
		})
	}
}

func TestResolveExtractWorkerPlanAutoPendingBatchCapRoundsDownToCandidate(t *testing.T) {
	t.Parallel()

	plan := resolveExtractWorkerPlan(
		"pdf",
		"ocr-heavy",
		config.DefaultExtractBatchWorkersSetting(),
		8,
		3,
		hostExtractObservation{
			PerfCores:     8,
			PhysicalCores: 10,
			TotalMemBytes: 16 << 30,
			FreeMemBytes:  8 << 30,
		},
		30000,
	)

	if plan.EffectiveAutoCap != 3 {
		t.Fatalf("EffectiveAutoCap = %d, want 3", plan.EffectiveAutoCap)
	}
	if !slices.Equal(plan.AutoCandidates, []int{1, 2}) {
		t.Fatalf("AutoCandidates = %v, want [1 2]", plan.AutoCandidates)
	}
	if plan.Used != 2 {
		t.Fatalf("Used = %d, want 2", plan.Used)
	}
}

func setupCfgForTest(t *testing.T) {
	t.Helper()
	cfg = config.DefaultConfig()
	t.Cleanup(func() { cfg = nil })
}

func TestExtractBatchesConcurrentAllSucceed(t *testing.T) {
	setupCfgForTest(t)

	dir := t.TempDir()
	jobDir := filepath.Join(dir, "job")
	pagesDir := filepath.Join(jobDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o750); err != nil {
		t.Fatal(err)
	}

	mock := &mockExtractor{
		extractPagesFn: func(_ context.Context, req extraction.ExtractPagesRequest) (*extraction.ExtractPagesResponse, error) {
			// Write a dummy JSONL file at OutputPath
			content := fmt.Sprintf("{\"page_num\":%d}\n", req.PageStart)
			if err := os.WriteFile(req.OutputPath, []byte(content), 0o600); err != nil {
				return nil, err
			}
			return &extraction.ExtractPagesResponse{
				PagesWritten: req.PageEnd - req.PageStart + 1,
				BatchMS:      10,
				Stats:        extraction.Stats{},
			}, nil
		},
	}

	manifest := &extractJobManifest{JobID: "test", Status: "extracting"}
	pending := []pageBatchRange{
		{PageStart: 1, PageEnd: 16},
		{PageStart: 17, PageEnd: 32},
		{PageStart: 33, PageEnd: 48},
		{PageStart: 49, PageEnd: 64},
	}

	ctx := context.Background()
	err := extractBatchesConcurrent(ctx, mock, "/tmp/test.pdf", "pdf", jobDir, manifest, pending, 2)
	if err != nil {
		t.Fatalf("extractBatchesConcurrent: %v", err)
	}
	if len(manifest.Completed) != 4 {
		t.Fatalf("completed batches = %d, want 4", len(manifest.Completed))
	}
}

func TestExtractBatchesConcurrentOneFailsCancelsRest(t *testing.T) {
	setupCfgForTest(t)
	// Use short timeout to speed up test with backoff
	cfg.Timeouts.WorkerBatch = 5 * time.Second

	dir := t.TempDir()
	jobDir := filepath.Join(dir, "job")
	pagesDir := filepath.Join(jobDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o750); err != nil {
		t.Fatal(err)
	}

	mock := &mockExtractor{
		extractPagesFn: func(_ context.Context, req extraction.ExtractPagesRequest) (*extraction.ExtractPagesResponse, error) {
			if req.PageStart == 17 {
				return nil, fmt.Errorf("simulated failure for batch starting at page 17")
			}
			content := fmt.Sprintf("{\"page_num\":%d}\n", req.PageStart)
			if err := os.WriteFile(req.OutputPath, []byte(content), 0o600); err != nil {
				return nil, err
			}
			return &extraction.ExtractPagesResponse{
				PagesWritten: req.PageEnd - req.PageStart + 1,
				BatchMS:      10,
			}, nil
		},
	}

	manifest := &extractJobManifest{JobID: "test", Status: "extracting"}
	pending := []pageBatchRange{
		{PageStart: 1, PageEnd: 16},
		{PageStart: 17, PageEnd: 32},
		{PageStart: 33, PageEnd: 48},
		{PageStart: 49, PageEnd: 64},
	}

	ctx := context.Background()
	err := extractBatchesConcurrent(ctx, mock, "/tmp/test.pdf", "pdf", jobDir, manifest, pending, 2)
	if err == nil {
		t.Fatal("expected error from failed batch")
	}
	if !strings.Contains(err.Error(), "17-32") {
		t.Fatalf("error = %q, want mention of pages 17-32", err)
	}
}

func TestExtractBatchesConcurrentContextCancel(t *testing.T) {
	setupCfgForTest(t)

	dir := t.TempDir()
	jobDir := filepath.Join(dir, "job")
	pagesDir := filepath.Join(jobDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o750); err != nil {
		t.Fatal(err)
	}

	mock := &mockExtractor{
		extractPagesFn: func(ctx context.Context, req extraction.ExtractPagesRequest) (*extraction.ExtractPagesResponse, error) {
			// Slow extraction to give time for cancellation
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
			content := fmt.Sprintf("{\"page_num\":%d}\n", req.PageStart)
			if err := os.WriteFile(req.OutputPath, []byte(content), 0o600); err != nil {
				return nil, err
			}
			return &extraction.ExtractPagesResponse{PagesWritten: 1, BatchMS: 50}, nil
		},
	}

	manifest := &extractJobManifest{JobID: "test", Status: "extracting"}
	pending := []pageBatchRange{
		{PageStart: 1, PageEnd: 16},
		{PageStart: 17, PageEnd: 32},
		{PageStart: 33, PageEnd: 48},
		{PageStart: 49, PageEnd: 64},
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()

	err := extractBatchesConcurrent(ctx, mock, "/tmp/test.pdf", "pdf", jobDir, manifest, pending, 2)
	// Should complete without deadlock. Error may or may not be nil depending on timing.
	_ = err
}
