package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInitCommand(t *testing.T) {
	// Create temp directories
	configDir := t.TempDir()
	dataDir := t.TempDir()

	// Override environment
	os.Setenv("STARBASE_CONFIG_DIR", configDir)
	os.Setenv("STARBASE_DATA_DIR", dataDir)
	defer os.Unsetenv("STARBASE_CONFIG_DIR")
	defer os.Unsetenv("STARBASE_DATA_DIR")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// Verify directories created
	expectedDirs := []string{
		configDir,
		dataDir,
		filepath.Join(dataDir, "clones"),
		filepath.Join(dataDir, "cache"),
	}
	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory not created: %s", dir)
		}
	}

	// Verify files created
	expectedFiles := []string{
		filepath.Join(configDir, "config.yaml"),
		filepath.Join(configDir, "manifest.yaml"),
		filepath.Join(dataDir, "starbase.db"),
	}
	for _, file := range expectedFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Errorf("File not created: %s", file)
		}
	}

	// Verify output
	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("initialized successfully")) {
		t.Errorf("Output missing success message: %s", output)
	}
}

func TestInitIdempotent(t *testing.T) {
	configDir := t.TempDir()
	dataDir := t.TempDir()

	os.Setenv("STARBASE_CONFIG_DIR", configDir)
	os.Setenv("STARBASE_DATA_DIR", dataDir)
	defer os.Unsetenv("STARBASE_CONFIG_DIR")
	defer os.Unsetenv("STARBASE_DATA_DIR")

	// Run init twice
	for i := 0; i < 2; i++ {
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetErr(buf)
		rootCmd.SetArgs([]string{"init"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("init command (run %d) failed: %v", i+1, err)
		}
	}

	// Verify still works
	if _, err := os.Stat(filepath.Join(dataDir, "starbase.db")); os.IsNotExist(err) {
		t.Error("Database not found after second init")
	}
}
