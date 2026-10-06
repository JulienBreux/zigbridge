package fixture

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed embedded/*.yaml
var embeddedFixturesFS embed.FS

// Registry stores and indexes parsed device definitions.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]*DeviceDefinition
}

// NewRegistry initializes an empty fixture registry.
func NewRegistry() *Registry {
	return &Registry{
		definitions: make(map[string]*DeviceDefinition),
	}
}

// LoadEmbedded loads built-in device definitions bundled into the binary.
func (r *Registry) LoadEmbedded() error {
	entries, err := fs.ReadDir(embeddedFixturesFS, "embedded")
	if err != nil {
		return fmt.Errorf("failed to read embedded fixtures directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := embeddedFixturesFS.ReadFile("embedded/" + entry.Name())
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", entry.Name(), err)
		}

		var def DeviceDefinition
		if err := yaml.Unmarshal(data, &def); err != nil {
			return fmt.Errorf("failed to parse embedded file %s: %w", entry.Name(), err)
		}

		if err := r.Register(&def); err != nil {
			return fmt.Errorf("failed to register embedded definition %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// LoadFromDir loads all .yaml and .yml files from a specified filesystem directory.
func (r *Registry) LoadFromDir(dir string) error {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) || (err == nil && !info.IsDir()) {
		return nil // directory not present, gracefully skip
	}
	if err != nil {
		return fmt.Errorf("failed to stat fixture directory %s: %w", dir, err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read fixture directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read fixture file %s: %w", filePath, err)
		}

		var def DeviceDefinition
		if err := yaml.Unmarshal(data, &def); err != nil {
			return fmt.Errorf("failed to parse fixture file %s: %w", filePath, err)
		}

		if err := r.Register(&def); err != nil {
			return fmt.Errorf("failed to register fixture %s: %w", filePath, err)
		}
	}

	return nil
}

// Register validates and indexes a definition by its model name and zigbee model aliases.
func (r *Registry) Register(def *DeviceDefinition) error {
	if def == nil {
		return fmt.Errorf("cannot register nil device definition")
	}
	if err := def.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.definitions[strings.ToLower(def.Device.Model)] = def
	for _, zm := range def.Device.ZigbeeModels {
		r.definitions[strings.ToLower(zm)] = def
	}

	return nil
}

// Get finds a device definition by model identifier or alias (case-insensitive).
func (r *Registry) Get(model string) (*DeviceDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.definitions[strings.ToLower(model)]
	return def, ok
}

// List returns all unique registered device definitions.
func (r *Registry) List() []*DeviceDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	var list []*DeviceDefinition
	for _, def := range r.definitions {
		if !seen[def.Device.Model] {
			seen[def.Device.Model] = true
			list = append(list, def)
		}
	}
	return list
}
