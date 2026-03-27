package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/dora56/refloom/internal/config"
)

const (
	autoExtractMinTotalMemBytes = 8 << 30
	autoExtractMinFreeMemBytes  = 1 << 30
	autoExtractCritFreeMemBytes = 256 << 20
	autoWarmupBatchCount        = 2
)

var (
	autoExtractTierACandidates = []int{1, 2, 4}
	autoExtractTierBCandidates = []int{1, 2, 4, 6}
	autoExtractTierCCandidates = []int{1, 2, 4, 6, 8}
)

type hostExtractObservation struct {
	PerfCores     int
	PhysicalCores int
	TotalMemBytes uint64
	FreeMemBytes  uint64
}

type autoExtractTier struct {
	Name        string
	PerfCores   int
	BaselineCap int
	Candidates  []int
}

type extractWorkerPlan struct {
	Mode              string
	Requested         string
	Used              int
	Reason            string
	ConfiguredAutoCap int
	EffectiveAutoCap  int
	AutoTier          string
	AutoCandidates    []int
}

var sysctlOutput = func(name string) ([]byte, error) {
	return exec.Command("sysctl", "-n", name).Output() //nolint:gosec // sysctl key comes from fixed call sites in code
}

var vmStatOutput = func() ([]byte, error) {
	return exec.Command("vm_stat").Output()
}

func resolveExtractWorkerPlan(
	format, mode string,
	setting config.ExtractBatchWorkersSetting,
	autoMaxWorkers int,
	pendingBatches int,
	host hostExtractObservation,
	warmupAvgMS int64,
) extractWorkerPlan {
	plan := extractWorkerPlan{
		Mode:      setting.Mode(),
		Requested: setting.RequestedString(),
		Used:      1,
		Reason:    "fixed workers requested",
	}
	if !isParallelExtractCandidate(format, mode) {
		if setting.Auto {
			plan.ConfiguredAutoCap = normalizeAutoExtractCap(autoMaxWorkers)
			plan.EffectiveAutoCap = 1
			plan.AutoTier = "none"
			plan.AutoCandidates = []int{1}
			plan.Reason = "tier=none selected=1 reason=non-ocr-heavy"
			return plan
		}
		plan.Reason = "parallel workers disabled for non-OCR-heavy PDF"
		return plan
	}
	if !setting.Auto {
		plan.Used = max(1, min(setting.Fixed, max(pendingBatches, 1)))
		plan.Reason = "fixed workers requested"
		return plan
	}
	plan.ConfiguredAutoCap = normalizeAutoExtractCap(autoMaxWorkers)

	tier := classifyAutoExtractTier(host)
	plan.AutoTier = tier.Name
	hostCap, memoryAdjustments := autoExtractHostCap(tier, plan.ConfiguredAutoCap, host)
	plan.EffectiveAutoCap = min(hostCap, max(pendingBatches, 1))
	plan.AutoCandidates = filterAutoCandidates(tier.Candidates, plan.EffectiveAutoCap)

	if pendingBatches <= 1 {
		plan.Used = 1
		plan.Reason = buildAutoWorkerReason(
			tier,
			host,
			plan.ConfiguredAutoCap,
			plan.EffectiveAutoCap,
			plan.Used,
			warmupAvgMS,
			pendingBatches,
			memoryAdjustments,
			"insufficient-pending-batches",
		)
		return plan
	}

	target := targetAutoWorkerCount(warmupAvgMS)
	plan.Used = roundDownAutoCandidate(plan.AutoCandidates, target)
	if host.FreeMemBytes > 0 && host.FreeMemBytes < autoExtractMinFreeMemBytes && host.FreeMemBytes >= autoExtractCritFreeMemBytes {
		plan.Used = downshiftAutoCandidate(plan.AutoCandidates, plan.Used)
		memoryAdjustments = append(memoryAdjustments, "free_mem_lt_1gib")
	}
	if plan.Used < 1 {
		plan.Used = 1
	}
	plan.Reason = buildAutoWorkerReason(
		tier,
		host,
		plan.ConfiguredAutoCap,
		plan.EffectiveAutoCap,
		plan.Used,
		warmupAvgMS,
		pendingBatches,
		memoryAdjustments,
		"warm-up",
	)
	return plan
}

func isParallelExtractCandidate(format, mode string) bool {
	return strings.EqualFold(format, "pdf") && strings.EqualFold(mode, "ocr-heavy")
}

func normalizeAutoExtractCap(configured int) int {
	if configured <= 0 {
		return config.DefaultExtractAutoMaxWorkers()
	}
	return configured
}

func classifyAutoExtractTier(host hostExtractObservation) autoExtractTier {
	perfCores := host.PerfCores
	if perfCores <= 0 {
		perfCores = host.PhysicalCores
	}
	if perfCores <= 0 {
		perfCores = runtime.NumCPU()
	}
	if perfCores <= 0 {
		perfCores = 1
	}
	switch {
	case perfCores <= 4:
		return autoExtractTier{
			Name:        "base",
			PerfCores:   perfCores,
			BaselineCap: 4,
			Candidates:  autoExtractTierACandidates,
		}
	case perfCores <= 7:
		return autoExtractTier{
			Name:        "pro",
			PerfCores:   perfCores,
			BaselineCap: 6,
			Candidates:  autoExtractTierBCandidates,
		}
	default:
		return autoExtractTier{
			Name:        "max",
			PerfCores:   perfCores,
			BaselineCap: 8,
			Candidates:  autoExtractTierCCandidates,
		}
	}
}

func autoExtractHostCap(tier autoExtractTier, configuredCap int, host hostExtractObservation) (int, []string) {
	effectiveCap := min(tier.BaselineCap, normalizeAutoExtractCap(configuredCap))
	memoryAdjustments := []string{}
	if host.TotalMemBytes > 0 && host.TotalMemBytes < autoExtractMinTotalMemBytes {
		effectiveCap = min(effectiveCap, 2)
		memoryAdjustments = append(memoryAdjustments, "total_mem_lt_8gib")
	}
	if host.FreeMemBytes > 0 && host.FreeMemBytes < autoExtractCritFreeMemBytes {
		effectiveCap = 1
		memoryAdjustments = append(memoryAdjustments, "free_mem_lt_256mib")
	}
	return max(1, effectiveCap), memoryAdjustments
}

func filterAutoCandidates(candidates []int, maxWorker int) []int {
	filtered := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate <= maxWorker {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) == 0 {
		return []int{1}
	}
	return filtered
}

func targetAutoWorkerCount(avgBatchMS int64) int {
	switch {
	case avgBatchMS < 1500:
		return 1
	case avgBatchMS < 5000:
		return 2
	case avgBatchMS < 10000:
		return 4
	case avgBatchMS < 20000:
		return 6
	default:
		return 8
	}
}

func roundDownAutoCandidate(candidates []int, target int) int {
	selected := 1
	for _, candidate := range candidates {
		if candidate > target {
			break
		}
		selected = candidate
	}
	return selected
}

func downshiftAutoCandidate(candidates []int, selected int) int {
	previous := 1
	for _, candidate := range candidates {
		if candidate >= selected {
			return previous
		}
		previous = candidate
	}
	return previous
}

func joinAutoCandidates(candidates []int) string {
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		parts = append(parts, strconv.Itoa(candidate))
	}
	return strings.Join(parts, ",")
}

func buildAutoWorkerReason(
	tier autoExtractTier,
	host hostExtractObservation,
	configuredCap, effectiveCap, selected int,
	avgBatchMS int64,
	pendingBatches int,
	memoryAdjustments []string,
	reason string,
) string {
	parts := []string{
		fmt.Sprintf("tier=%s", tier.Name),
		fmt.Sprintf("perf_cores=%d", tier.PerfCores),
		fmt.Sprintf("total_mem_gb=%s", gibibytesString(host.TotalMemBytes)),
		fmt.Sprintf("free_mem_gb=%s", gibibytesString(host.FreeMemBytes)),
		fmt.Sprintf("avg_batch_ms=%d", avgBatchMS),
		fmt.Sprintf("configured_cap=%d", configuredCap),
		fmt.Sprintf("effective_cap=%d", effectiveCap),
		fmt.Sprintf("pending_batches=%d", pendingBatches),
		fmt.Sprintf("selected=%d", selected),
		fmt.Sprintf("candidate_workers=%s", joinAutoCandidates(filterAutoCandidates(tier.Candidates, effectiveCap))),
	}
	if len(memoryAdjustments) > 0 {
		parts = append(parts, fmt.Sprintf("memory_adjusted=%s", strings.Join(memoryAdjustments, "+")))
	}
	parts = append(parts, fmt.Sprintf("reason=%s", reason))
	return strings.Join(parts, " ")
}

func observeHostExtractCapacity() hostExtractObservation {
	host := hostExtractObservation{
		PhysicalCores: runtime.NumCPU(),
	}
	if value, err := readSysctlInt("hw.perflevel0.physicalcpu"); err == nil && value > 0 {
		host.PerfCores = int(value)
	}
	if value, err := readSysctlInt("hw.physicalcpu"); err == nil && value > 0 {
		host.PhysicalCores = int(value)
	}
	if value, err := readSysctlInt("hw.memsize"); err == nil && value > 0 {
		host.TotalMemBytes = uint64(value)
	}
	if output, err := vmStatOutput(); err == nil {
		if freeBytes, parseErr := parseVMStatFreeBytes(output); parseErr == nil {
			host.FreeMemBytes = freeBytes
		}
	}
	return host
}

func readSysctlInt(name string) (int64, error) {
	output, err := sysctlOutput(name)
	if err != nil {
		return 0, err
	}
	return parseSysctlInt64(output)
}

func parseSysctlInt64(raw []byte) (int64, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return 0, fmt.Errorf("empty sysctl output")
	}
	return strconv.ParseInt(trimmed, 10, 64)
}

func parseVMStatFreeBytes(raw []byte) (uint64, error) {
	lines := strings.Split(string(raw), "\n")
	var pageSize uint64
	var freePages uint64
	var speculativePages uint64
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "page size of") {
			_, after, ok := strings.Cut(line, "page size of")
			if !ok {
				continue
			}
			rest := after
			rest = strings.TrimSpace(strings.TrimSuffix(rest, "bytes)"))
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				continue
			}
			value, err := strconv.ParseUint(fields[0], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse vm_stat page size: %w", err)
			}
			pageSize = value
			continue
		}
		switch {
		case strings.HasPrefix(line, "Pages free:"):
			value, err := parseVMStatPageCount(line)
			if err != nil {
				return 0, err
			}
			freePages = value
		case strings.HasPrefix(line, "Pages speculative:"):
			value, err := parseVMStatPageCount(line)
			if err != nil {
				return 0, err
			}
			speculativePages = value
		}
	}
	if pageSize == 0 {
		return 0, fmt.Errorf("vm_stat page size not found")
	}
	return (freePages + speculativePages) * pageSize, nil
}

func parseVMStatPageCount(line string) (uint64, error) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid vm_stat line %q", line)
	}
	value := strings.TrimSpace(parts[1])
	value = strings.TrimSuffix(value, ".")
	value = strings.ReplaceAll(value, ".", "")
	value = strings.ReplaceAll(value, ",", "")
	return strconv.ParseUint(strings.TrimSpace(value), 10, 64)
}

func gibibytesString(bytesValue uint64) string {
	if bytesValue == 0 {
		return "unknown"
	}
	value := float64(bytesValue) / float64(1<<30)
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", value), "0"), ".")
}

func doctorAutoExtractDetail(host hostExtractObservation, configuredCap int) string {
	tier := classifyAutoExtractTier(host)
	hostCap, memoryAdjustments := autoExtractHostCap(tier, configuredCap, host)
	candidates := filterAutoCandidates(tier.Candidates, hostCap)
	memory := "none"
	if len(memoryAdjustments) > 0 {
		memory = strings.Join(memoryAdjustments, "+")
	}
	return fmt.Sprintf(
		"perf_cores=%d physical_cores=%d total_mem_gb=%s free_mem_gb=%s auto_tier=%s configured_auto_cap=%d effective_auto_cap=%d candidate_workers=%s memory_adjustments=%s gpu=not used in auto v1",
		tier.PerfCores,
		host.PhysicalCores,
		gibibytesString(host.TotalMemBytes),
		gibibytesString(host.FreeMemBytes),
		tier.Name,
		normalizeAutoExtractCap(configuredCap),
		hostCap,
		joinAutoCandidates(candidates),
		memory,
	)
}
