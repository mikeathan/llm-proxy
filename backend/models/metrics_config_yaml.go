package models

import "gopkg.in/yaml.v3"

// Backward compatibility for settings.yml GPU metrics keys.
//
// Before yaml tags were added to MetricsConfig/GPUConfig, the yaml store
// persisted field-name-derived keys ("gpusampleintervalsec", "gpusmoothingalpha",
// "sysfspath"). The documented snake_case keys ("gpu_sample_interval_seconds",
// "gpu_smoothing_alpha", "sysfs_path") are now the canonical format (yaml tags
// above); these custom unmarshalers accept BOTH so existing files keep loading
// after an upgrade. Legacy values apply only when the canonical key is absent
// (field still zero), so a file re-saved in the new format always wins. The
// next store write naturally migrates a legacy file to the canonical format.

// UnmarshalYAML decodes the canonical yaml tags, then falls back to the legacy
// field-name-derived keys for any field the canonical keys did not populate.
func (m *MetricsConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain MetricsConfig
	if err := value.Decode((*plain)(m)); err != nil {
		return err
	}
	if value.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(value.Content); i += 2 {
		key := value.Content[i].Value
		switch key {
		case "gpusampleintervalsec":
			if m.GPUSampleIntervalSec == 0 {
				if err := value.Content[i+1].Decode(&m.GPUSampleIntervalSec); err != nil {
					return err
				}
			}
		case "gpusmoothingalpha":
			if m.GPUSmoothingAlpha == 0 {
				if err := value.Content[i+1].Decode(&m.GPUSmoothingAlpha); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// UnmarshalYAML decodes the canonical yaml tags, then accepts the legacy
// "sysfspath" key for SysfsPath.
func (g *GPUConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain GPUConfig
	if err := value.Decode((*plain)(g)); err != nil {
		return err
	}
	if value.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(value.Content); i += 2 {
		if value.Content[i].Value == "sysfspath" && g.SysfsPath == "" {
			if err := value.Content[i+1].Decode(&g.SysfsPath); err != nil {
				return err
			}
		}
	}
	return nil
}
