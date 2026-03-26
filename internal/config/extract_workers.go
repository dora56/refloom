package config

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const extractBatchWorkersAuto = "auto"
const defaultExtractAutoMaxWorkers = 8

// ExtractBatchWorkersSetting accepts either "auto" or a positive integer.
type ExtractBatchWorkersSetting struct {
	Auto  bool
	Fixed int
}

func DefaultExtractBatchWorkersSetting() ExtractBatchWorkersSetting {
	return ExtractBatchWorkersSetting{Auto: true}
}

func DefaultExtractAutoMaxWorkers() int {
	return defaultExtractAutoMaxWorkers
}

func ParseExtractBatchWorkersSetting(raw string) (ExtractBatchWorkersSetting, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return ExtractBatchWorkersSetting{}, fmt.Errorf("empty extract batch workers value")
	}
	if trimmed == extractBatchWorkersAuto {
		return DefaultExtractBatchWorkersSetting(), nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return ExtractBatchWorkersSetting{}, fmt.Errorf("parse extract batch workers: %w", err)
	}
	if value <= 0 {
		return ExtractBatchWorkersSetting{}, fmt.Errorf("extract batch workers must be positive or auto")
	}
	return ExtractBatchWorkersSetting{Fixed: value}, nil
}

func (s ExtractBatchWorkersSetting) Mode() string {
	if s.Auto {
		return extractBatchWorkersAuto
	}
	return "fixed"
}

func (s ExtractBatchWorkersSetting) RequestedString() string {
	if s.Auto {
		return extractBatchWorkersAuto
	}
	return strconv.Itoa(s.Fixed)
}

func (s ExtractBatchWorkersSetting) RequestedCount() int {
	if s.Auto {
		return 0
	}
	return s.Fixed
}

func (s ExtractBatchWorkersSetting) MarshalYAML() (any, error) {
	if s.Auto {
		return extractBatchWorkersAuto, nil
	}
	return s.Fixed, nil
}

func (s *ExtractBatchWorkersSetting) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		setting, err := ParseExtractBatchWorkersSetting(node.Value)
		if err != nil {
			return err
		}
		*s = setting
		return nil
	default:
		return fmt.Errorf("extract batch workers must be a scalar")
	}
}
