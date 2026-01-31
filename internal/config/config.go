package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Version int    `mapstructure:"version"`
	DataDir string `mapstructure:"data_dir"`

	Clone   CloneConfig   `mapstructure:"clone"`
	Sync    SyncConfig    `mapstructure:"sync"`
	Index   IndexConfig   `mapstructure:"index"`
	Search  SearchConfig  `mapstructure:"search"`
	Ranking RankingConfig `mapstructure:"ranking"`
	Forges  ForgesConfig  `mapstructure:"forges"`
	Export  ExportConfig  `mapstructure:"export"`
	Prune   PruneConfig   `mapstructure:"prune"`
}

type CloneConfig struct {
	Depth          int  `mapstructure:"depth"`
	SingleBranch   bool `mapstructure:"single_branch"`
	SkipSubmodules bool `mapstructure:"skip_submodules"`
	SkipLFS        bool `mapstructure:"skip_lfs"`
}

type SyncConfig struct {
	DefaultWindow   string `mapstructure:"default_window"`
	CloneMissing    bool   `mapstructure:"clone_missing"`
	ClonePrivate    bool   `mapstructure:"clone_private"`
	CloneArchived   bool   `mapstructure:"clone_archived"`
	MaxReposPerSync int    `mapstructure:"max_repos_per_sync"`
	ResetOnConflict bool   `mapstructure:"reset_on_conflict"`
	Concurrency     int    `mapstructure:"concurrency"`
}

type IndexConfig struct {
	Readme          bool     `mapstructure:"readme"`
	HighSignalFiles []string `mapstructure:"high_signal_files"`
	MaxFileSizeKB   int      `mapstructure:"max_file_size_kb"`
}

type SearchConfig struct {
	Engine            string `mapstructure:"engine"`
	EmbeddingModel    string `mapstructure:"embedding_model"`
	EmbeddingProvider string `mapstructure:"embedding_provider"`
	DefaultLimit      int    `mapstructure:"default_limit"`
}

type RankingConfig struct {
	StarRecencyWeight   float64 `mapstructure:"star_recency_weight"`
	PushRecencyWeight   float64 `mapstructure:"push_recency_weight"`
	LanguageMatchWeight float64 `mapstructure:"language_match_weight"`
}

type ForgesConfig struct {
	GitHub GitHubConfig `mapstructure:"github"`
	GitLab GitLabConfig `mapstructure:"gitlab"`
}

type GitHubConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

type GitLabConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Host    string `mapstructure:"host"`
}

type ExportConfig struct {
	DefaultFormat string `mapstructure:"default_format"`
	IncludeReadme bool   `mapstructure:"include_readme"`
}

type PruneConfig struct {
	MaxTotalSizeGB int  `mapstructure:"max_total_size_gb"`
	MaxRepoAgeDays int  `mapstructure:"max_repo_age_days"`
	KeepPinned     bool `mapstructure:"keep_pinned"`
}

// DefaultWindow returns the sync window as a duration
func (s *SyncConfig) DefaultWindowDuration() (time.Duration, error) {
	return parseDuration(s.DefaultWindow)
}

func parseDuration(s string) (time.Duration, error) {
	if len(s) == 0 {
		return 30 * 24 * time.Hour, nil // default 30 days
	}

	unit := s[len(s)-1]
	value := s[:len(s)-1]

	var multiplier time.Duration
	switch unit {
	case 'd':
		multiplier = 24 * time.Hour
	case 'w':
		multiplier = 7 * 24 * time.Hour
	case 'h':
		multiplier = time.Hour
	default:
		return 0, fmt.Errorf("unknown duration unit: %c", unit)
	}

	var n int
	if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}

	return time.Duration(n) * multiplier, nil
}

// Paths holds the resolved directory paths
type Paths struct {
	ConfigDir string
	DataDir   string
	ClonesDir string
	CacheDir  string
	DBPath    string
}

// DefaultConfigDir returns the default config directory
func DefaultConfigDir() string {
	if dir := os.Getenv("STARBASE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "starbase")
}

// DefaultDataDir returns the default data directory
func DefaultDataDir() string {
	if dir := os.Getenv("STARBASE_DATA_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "starbase")
}

// ResolvePaths returns resolved paths based on config
func ResolvePaths(cfg *Config) Paths {
	dataDir := cfg.DataDir
	if dataDir == "" || dataDir == "~/.local/share/starbase" {
		dataDir = DefaultDataDir()
	}
	dataDir = expandHome(dataDir)

	return Paths{
		ConfigDir: DefaultConfigDir(),
		DataDir:   dataDir,
		ClonesDir: filepath.Join(dataDir, "clones"),
		CacheDir:  filepath.Join(dataDir, "cache"),
		DBPath:    filepath.Join(dataDir, "starbase.db"),
	}
}

func expandHome(path string) string {
	if len(path) > 1 && path[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// Load reads the config from file and environment
func Load(configDir string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Config file
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(configDir)

	// Environment variables
	v.SetEnvPrefix("STARBASE")
	v.AutomaticEnv()

	// Read config file (optional - use defaults if missing)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("version", 1)
	v.SetDefault("data_dir", "~/.local/share/starbase")

	// Clone defaults
	v.SetDefault("clone.depth", 1)
	v.SetDefault("clone.single_branch", true)
	v.SetDefault("clone.skip_submodules", true)
	v.SetDefault("clone.skip_lfs", true)

	// Sync defaults
	v.SetDefault("sync.default_window", "30d")
	v.SetDefault("sync.clone_missing", true)
	v.SetDefault("sync.clone_private", false)
	v.SetDefault("sync.clone_archived", false)
	v.SetDefault("sync.max_repos_per_sync", 0) // 0 = no limit
	v.SetDefault("sync.reset_on_conflict", true)
	v.SetDefault("sync.concurrency", 4)

	// Index defaults
	v.SetDefault("index.readme", true)
	v.SetDefault("index.high_signal_files", []string{
		"go.mod",
		"package.json",
		"Cargo.toml",
		"pyproject.toml",
		"Dockerfile",
		"docker-compose.yml",
	})
	v.SetDefault("index.max_file_size_kb", 100)

	// Search defaults
	v.SetDefault("search.engine", "bm25")
	v.SetDefault("search.embedding_model", "nomic-embed-text")
	v.SetDefault("search.embedding_provider", "ollama")
	v.SetDefault("search.default_limit", 20)

	// Ranking defaults
	v.SetDefault("ranking.star_recency_weight", 0.2)
	v.SetDefault("ranking.push_recency_weight", 0.1)
	v.SetDefault("ranking.language_match_weight", 0.15)

	// Forges defaults
	v.SetDefault("forges.github.enabled", true)
	v.SetDefault("forges.gitlab.enabled", false)
	v.SetDefault("forges.gitlab.host", "gitlab.com")

	// Export defaults
	v.SetDefault("export.default_format", "paths")
	v.SetDefault("export.include_readme", false)

	// Prune defaults
	v.SetDefault("prune.max_total_size_gb", 50)
	v.SetDefault("prune.max_repo_age_days", 365)
	v.SetDefault("prune.keep_pinned", true)
}
