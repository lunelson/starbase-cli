package forge

import (
	"context"
	"testing"
)

func TestForgeInterfaceExists(t *testing.T) {
	// This test verifies the interface is properly defined
	// by ensuring a mock can implement it
	var _ Forge = (*mockForge)(nil)
}

// mockForge is a test implementation
type mockForge struct{}

func (m *mockForge) Name() string { return "mock" }

func (m *mockForge) ListStars(ctx context.Context, opts ListOptions) (*ListResult, error) {
	return &ListResult{}, nil
}

func (m *mockForge) GetRepo(ctx context.Context, owner, name string) (*Repository, error) {
	return &Repository{}, nil
}

func (m *mockForge) GetReadme(ctx context.Context, owner, name string) (string, error) {
	return "", nil
}
