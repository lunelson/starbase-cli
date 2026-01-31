package forge

import (
	"testing"
)

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantHost  string
		wantOwner string
		wantRepo  string
		wantID    string
		wantErr   bool
	}{
		{
			name:      "short form",
			input:     "charmbracelet/bubbletea",
			wantHost:  "github.com",
			wantOwner: "charmbracelet",
			wantRepo:  "bubbletea",
			wantID:    "github.com:charmbracelet/bubbletea",
		},
		{
			name:      "short form with spaces",
			input:     "  owner/repo  ",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantID:    "github.com:owner/repo",
		},
		{
			name:      "https URL",
			input:     "https://github.com/owner/repo",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantID:    "github.com:owner/repo",
		},
		{
			name:      "https URL with .git",
			input:     "https://github.com/owner/repo.git",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantID:    "github.com:owner/repo",
		},
		{
			name:      "https URL with extra path",
			input:     "https://github.com/owner/repo/tree/main/src/file.ts",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantID:    "github.com:owner/repo",
		},
		{
			name:      "https URL with query params",
			input:     "https://github.com/owner/repo?tab=readme",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantID:    "github.com:owner/repo",
		},
		{
			name:      "https URL with fragment",
			input:     "https://github.com/owner/repo#installation",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantID:    "github.com:owner/repo",
		},
		{
			name:      "SSH URL",
			input:     "git@github.com:owner/repo.git",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantID:    "github.com:owner/repo",
		},
		{
			name:      "SSH URL without .git",
			input:     "git@github.com:owner/repo",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantID:    "github.com:owner/repo",
		},
		{
			name:      "GitLab https",
			input:     "https://gitlab.com/group/project",
			wantHost:  "gitlab.com",
			wantOwner: "group",
			wantRepo:  "project",
			wantID:    "gitlab.com:group/project",
		},
		{
			name:      "GitLab SSH",
			input:     "git@gitlab.com:group/project.git",
			wantHost:  "gitlab.com",
			wantOwner: "group",
			wantRepo:  "project",
			wantID:    "gitlab.com:group/project",
		},
		{
			name:      "custom host",
			input:     "https://git.company.com/team/internal-tool",
			wantHost:  "git.company.com",
			wantOwner: "team",
			wantRepo:  "internal-tool",
			wantID:    "git.company.com:team/internal-tool",
		},
		{
			name:    "invalid - single segment",
			input:   "repo",
			wantErr: true,
		},
		{
			name:    "invalid - empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid - https without path",
			input:   "https://github.com/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRepoURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseRepoURL(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseRepoURL(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", got.Host, tt.wantHost)
			}
			if got.Owner != tt.wantOwner {
				t.Errorf("Owner = %q, want %q", got.Owner, tt.wantOwner)
			}
			if got.Name != tt.wantRepo {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantRepo)
			}
			if got.ID() != tt.wantID {
				t.Errorf("ID() = %q, want %q", got.ID(), tt.wantID)
			}
		})
	}
}

func TestHostForgeConversion(t *testing.T) {
	if got := HostFromForge("github"); got != "github.com" {
		t.Errorf("HostFromForge(github) = %q, want github.com", got)
	}
	if got := ForgeFromHost("github.com"); got != "github" {
		t.Errorf("ForgeFromHost(github.com) = %q, want github", got)
	}
}
