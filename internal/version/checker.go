package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// GitHubAPIURL is the GitHub API endpoint for releases
	githubReleasesURL = "https://api.github.com/repos/%s/%s/releases/latest"
	// requestTimeout is the timeout for HTTP requests
	requestTimeout = 5 * time.Second
)

// ReleaseInfo contains information about a GitHub release
type ReleaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Name    string `json:"name"`
}

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
	ReleaseName    string
}

// Checker checks for new versions on GitHub
type Checker struct {
	owner   string
	repo    string
	current string
	client  *http.Client
}

// NewChecker creates a new version checker
func NewChecker(owner, repo, currentVersion string) *Checker {
	return &Checker{
		owner:   owner,
		repo:    repo,
		current: normalizeVersion(currentVersion),
		client: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

// CheckForUpdate checks if a newer version is available.
// Returns nil if current version is up to date or if check fails.
// This is designed to fail silently - network errors should not impact the user.
func (c *Checker) CheckForUpdate(ctx context.Context) *UpdateInfo {
	release, err := c.fetchLatestRelease(ctx)
	if err != nil {
		return nil
	}

	latestVersion := normalizeVersion(release.TagName)
	if isNewerVersion(latestVersion, c.current) {
		return &UpdateInfo{
			CurrentVersion: c.current,
			LatestVersion:  latestVersion,
			ReleaseURL:     release.HTMLURL,
			ReleaseName:    release.Name,
		}
	}

	return nil
}

// fetchLatestRelease fetches the latest release info from GitHub API
func (c *Checker) fetchLatestRelease(ctx context.Context) (*ReleaseInfo, error) {
	url := fmt.Sprintf(githubReleasesURL, c.owner, c.repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "kportal-version-checker")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// normalizeVersion removes 'v' or 'V' prefix and trims whitespace
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

// isNewerVersion compares two semver-like versions.
// Returns true if latest is newer than current.
func isNewerVersion(latest, current string) bool {
	latestParts := parseVersion(latest)
	currentParts := parseVersion(current)

	// Compare each part
	for i := 0; i < len(latestParts) && i < len(currentParts); i++ {
		if latestParts[i] > currentParts[i] {
			return true
		}
		if latestParts[i] < currentParts[i] {
			return false
		}
	}

	// If all compared parts are equal, longer version is newer
	// e.g., 1.0.1 > 1.0
	return len(latestParts) > len(currentParts)
}

// parseVersion splits a version string into numeric parts
func parseVersion(v string) []int {
	// Remove any suffix like -beta, -rc1, etc.
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	result := make([]int, 0, len(parts))

	for _, p := range parts {
		var num int
		fmt.Sscanf(p, "%d", &num)
		result = append(result, num)
	}

	return result
}

// FormatUpdateMessage formats a user-friendly update notification
func (u *UpdateInfo) FormatUpdateMessage() string {
	return fmt.Sprintf("New version available: %s (current: %s) - %s",
		u.LatestVersion, u.CurrentVersion, u.ReleaseURL)
}
