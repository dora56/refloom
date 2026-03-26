package cli

import (
	"strings"
	"testing"
)

func TestParseSysctlInt64(t *testing.T) {
	t.Parallel()

	got, err := parseSysctlInt64([]byte(" 1234\n"))
	if err != nil {
		t.Fatalf("parseSysctlInt64: %v", err)
	}
	if got != 1234 {
		t.Fatalf("got %d, want 1234", got)
	}
}

func TestParseVMStatFreeBytes(t *testing.T) {
	t.Parallel()

	output := []byte(`Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               1024.
Pages speculative:                         512.
Pages active:                             1000.
`)

	got, err := parseVMStatFreeBytes(output)
	if err != nil {
		t.Fatalf("parseVMStatFreeBytes: %v", err)
	}
	want := uint64(1536 * 16384)
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

func TestClassifyAutoExtractTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		perfCores int
		wantTier  string
		wantCap   int
	}{
		{name: "base tier", perfCores: 4, wantTier: "base", wantCap: 4},
		{name: "pro tier", perfCores: 6, wantTier: "pro", wantCap: 6},
		{name: "max tier", perfCores: 8, wantTier: "max", wantCap: 8},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tier := classifyAutoExtractTier(hostExtractObservation{PerfCores: tc.perfCores})
			if tier.Name != tc.wantTier {
				t.Fatalf("Name = %q, want %q", tier.Name, tc.wantTier)
			}
			if tier.BaselineCap != tc.wantCap {
				t.Fatalf("BaselineCap = %d, want %d", tier.BaselineCap, tc.wantCap)
			}
		})
	}
}

func TestDoctorAutoExtractDetailIncludesTierAndCaps(t *testing.T) {
	t.Parallel()

	detail := doctorAutoExtractDetail(hostExtractObservation{
		PerfCores:     8,
		PhysicalCores: 10,
		TotalMemBytes: 16 << 30,
		FreeMemBytes:  2 << 30,
	}, 6)

	for _, want := range []string{
		"auto_tier=max",
		"configured_auto_cap=6",
		"effective_auto_cap=6",
		"candidate_workers=1,2,4,6",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail = %q, want substring %q", detail, want)
		}
	}
}

func TestParseVMStatFreeBytesEmptyPageSize(t *testing.T) {
	t.Parallel()

	// "page size of" followed by only whitespace should not panic
	output := []byte("Mach Virtual Memory Statistics: (page size of   )\nPages free:                               1024.\n")
	_, err := parseVMStatFreeBytes(output)
	if err == nil {
		t.Fatal("expected error for missing page size value")
	}
}

func TestAutoExtractObservationDegraded(t *testing.T) {
	t.Parallel()

	if !autoExtractObservationDegraded(hostExtractObservation{PerfCores: 0, TotalMemBytes: 16 << 30, FreeMemBytes: 2 << 30}) {
		t.Fatalf("expected degraded when perf cores are unavailable")
	}
	if !autoExtractObservationDegraded(hostExtractObservation{PerfCores: 8, TotalMemBytes: 0, FreeMemBytes: 2 << 30}) {
		t.Fatalf("expected degraded when total memory is unavailable")
	}
	if !autoExtractObservationDegraded(hostExtractObservation{PerfCores: 8, TotalMemBytes: 16 << 30, FreeMemBytes: 0}) {
		t.Fatalf("expected degraded when free memory is unavailable")
	}
	if autoExtractObservationDegraded(hostExtractObservation{PerfCores: 8, TotalMemBytes: 16 << 30, FreeMemBytes: 2 << 30}) {
		t.Fatalf("did not expect degraded for full observation")
	}
}
