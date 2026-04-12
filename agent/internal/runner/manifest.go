package runner

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Manifest represents a compliance bundle's manifest.yaml.
type Manifest struct {
	Name           string `yaml:"name"`
	Version        string `yaml:"version"`
	Framework      string `yaml:"framework"`
	RuntimeVersion string `yaml:"runtime_version"`
	TargetType     string `yaml:"target_type"`
	Entrypoint     string `yaml:"entrypoint"`
	Benchmark      struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
		CISID   string `yaml:"cis_id"`
	} `yaml:"benchmark"`
	Outputs struct {
		Format string `yaml:"format"`
	} `yaml:"outputs"`
}

// LoadManifest reads and parses the manifest.yaml from the given bundle path.
func LoadManifest(bundlePath string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(bundlePath, "manifest.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	if m.Entrypoint == "" {
		return nil, fmt.Errorf("manifest missing entrypoint")
	}

	return &m, nil
}
