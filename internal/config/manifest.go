package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Version     int                `yaml:"version"`
	Collections []Collection       `yaml:"collections"`
	Repos       []ManifestRepo     `yaml:"repos"`
}

type Collection struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Color       string `yaml:"color,omitempty"`
}

type ManifestRepo struct {
	Forge       string   `yaml:"forge"`
	Owner       string   `yaml:"owner"`
	Name        string   `yaml:"name"`
	ForgeID     string   `yaml:"forge_id,omitempty"`
	Collections []string `yaml:"collections,omitempty"`
	Notes       string   `yaml:"notes,omitempty"`
	Pinned      bool     `yaml:"pinned,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
}

// FullName returns "owner/name"
func (r *ManifestRepo) FullName() string {
	return fmt.Sprintf("%s/%s", r.Owner, r.Name)
}

// LoadManifest reads the manifest from the config directory
func LoadManifest(configDir string) (*Manifest, error) {
	path := filepath.Join(configDir, "manifest.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty manifest if file doesn't exist
			return &Manifest{Version: 1}, nil
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	if m.Version == 0 {
		m.Version = 1
	}

	return &m, nil
}

// SaveManifest writes the manifest to the config directory
func SaveManifest(configDir string, m *Manifest) error {
	path := filepath.Join(configDir, "manifest.yaml")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	return nil
}

// GetRepo finds a repo by forge and full name
func (m *Manifest) GetRepo(forge, owner, name string) *ManifestRepo {
	for i := range m.Repos {
		if m.Repos[i].Forge == forge && m.Repos[i].Owner == owner && m.Repos[i].Name == name {
			return &m.Repos[i]
		}
	}
	return nil
}

// GetRepoByForgeID finds a repo by forge and forge ID
func (m *Manifest) GetRepoByForgeID(forge, forgeID string) *ManifestRepo {
	for i := range m.Repos {
		if m.Repos[i].Forge == forge && m.Repos[i].ForgeID == forgeID {
			return &m.Repos[i]
		}
	}
	return nil
}

// AddRepo adds a new repo to the manifest
func (m *Manifest) AddRepo(repo ManifestRepo) {
	m.Repos = append(m.Repos, repo)
}

// RemoveRepo removes a repo by forge and full name
func (m *Manifest) RemoveRepo(forge, owner, name string) bool {
	for i := range m.Repos {
		if m.Repos[i].Forge == forge && m.Repos[i].Owner == owner && m.Repos[i].Name == name {
			m.Repos = append(m.Repos[:i], m.Repos[i+1:]...)
			return true
		}
	}
	return false
}

// GetCollection finds a collection by name
func (m *Manifest) GetCollection(name string) *Collection {
	for i := range m.Collections {
		if m.Collections[i].Name == name {
			return &m.Collections[i]
		}
	}
	return nil
}

// AddCollection adds a new collection
func (m *Manifest) AddCollection(c Collection) {
	m.Collections = append(m.Collections, c)
}

// ReposInCollection returns all repos that belong to a collection
func (m *Manifest) ReposInCollection(collectionName string) []ManifestRepo {
	var result []ManifestRepo
	for _, r := range m.Repos {
		for _, c := range r.Collections {
			if c == collectionName {
				result = append(result, r)
				break
			}
		}
	}
	return result
}
