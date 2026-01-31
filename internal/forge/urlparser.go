package forge

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const DefaultHost = "github.com"

type ParsedRepo struct {
	Host     string
	Owner    string
	Name     string
	CloneURL string
}

func (p ParsedRepo) ID() string {
	return fmt.Sprintf("%s:%s/%s", p.Host, p.Owner, p.Name)
}

func (p ParsedRepo) FullName() string {
	return fmt.Sprintf("%s/%s", p.Owner, p.Name)
}

var sshPattern = regexp.MustCompile(`^git@([^:]+):(.+)$`)

func ParseRepoURL(input string) (*ParsedRepo, error) {
	trimmed := strings.TrimSpace(input)

	if parsed := parseSSH(trimmed); parsed != nil {
		return parsed, nil
	}

	if parsed := parseHTTPS(trimmed); parsed != nil {
		return parsed, nil
	}

	if parsed := parseShortForm(trimmed); parsed != nil {
		return parsed, nil
	}

	return nil, fmt.Errorf(
		"invalid repo format: %s\nExpected: owner/repo, https://host/owner/repo, or git@host:owner/repo.git",
		input,
	)
}

func parseSSH(input string) *ParsedRepo {
	match := sshPattern.FindStringSubmatch(input)
	if match == nil {
		return nil
	}

	host := match[1]
	path := strings.Split(match[2], "?")[0]
	path = strings.Split(path, "#")[0]

	segments := strings.Split(path, "/")
	if len(segments) < 2 {
		return nil
	}

	owner := segments[0]
	repo := strings.TrimSuffix(segments[1], ".git")

	return &ParsedRepo{
		Host:     host,
		Owner:    owner,
		Name:     repo,
		CloneURL: fmt.Sprintf("git@%s:%s/%s.git", host, owner, repo),
	}
}

func parseHTTPS(input string) *ParsedRepo {
	if !strings.HasPrefix(input, "http://") && !strings.HasPrefix(input, "https://") {
		return nil
	}

	normalized := normalizeHTTPURL(input)
	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 2 {
		return nil
	}

	owner := segments[0]
	repo := strings.TrimSuffix(segments[1], ".git")

	return &ParsedRepo{
		Host:     parsed.Host,
		Owner:    owner,
		Name:     repo,
		CloneURL: fmt.Sprintf("%s://%s/%s/%s.git", parsed.Scheme, parsed.Host, owner, repo),
	}
}

func parseShortForm(input string) *ParsedRepo {
	parts := strings.Split(input, "/")
	if len(parts) != 2 {
		return nil
	}

	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSpace(parts[1])

	if owner == "" || repo == "" {
		return nil
	}

	if strings.ContainsAny(owner, ":@") || strings.ContainsAny(repo, ":@") {
		return nil
	}

	return &ParsedRepo{
		Host:     DefaultHost,
		Owner:    owner,
		Name:     repo,
		CloneURL: fmt.Sprintf("https://%s/%s/%s.git", DefaultHost, owner, repo),
	}
}

func normalizeHTTPURL(input string) string {
	parsed, err := url.Parse(input)
	if err != nil {
		return input
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 2 {
		return input
	}

	owner := segments[0]
	repo := segments[1]

	return fmt.Sprintf("%s://%s/%s/%s", parsed.Scheme, parsed.Host, owner, repo)
}

func HostFromForge(forge string) string {
	switch forge {
	case "github":
		return "github.com"
	case "gitlab":
		return "gitlab.com"
	default:
		return forge
	}
}

func ForgeFromHost(host string) string {
	switch host {
	case "github.com":
		return "github"
	case "gitlab.com":
		return "gitlab"
	default:
		return host
	}
}
