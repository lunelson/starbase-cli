package config

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
)

type PathTemplateData struct {
	Forge    string
	Owner    string
	Name     string
	FullName string
	Language string
}

func ExpandPathTemplate(tmpl string, data PathTemplateData) (string, error) {
	t, err := template.New("path").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	result := buf.String()
	result = sanitizePath(result)

	return result, nil
}

func ValidatePathTemplate(tmpl string) error {
	_, err := template.New("path").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}

	testData := PathTemplateData{
		Forge:    "github",
		Owner:    "test",
		Name:     "repo",
		FullName: "test/repo",
		Language: "Go",
	}

	if _, err := ExpandPathTemplate(tmpl, testData); err != nil {
		return err
	}

	return nil
}

func sanitizePath(path string) string {
	path = strings.ReplaceAll(path, "\x00", "")

	parts := strings.Split(path, string(filepath.Separator))
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
		if parts[i] == "" {
			parts[i] = "_"
		}
	}

	return filepath.Join(parts...)
}
