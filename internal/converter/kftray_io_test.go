package converter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/lukaszraczylo/kportal/internal/config"
)

// writeJSON writes v as JSON to a temp file in dir, returns the path.
func writeJSON(t *testing.T, dir string, name string, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
}

// ─── ConvertKFTrayToKPortal ──────────────────────────────────────────────────

func TestConvertKFTrayToKPortal_HappyPath(t *testing.T) {
	dir := t.TempDir()
	input := writeJSON(t, dir, "in.json", []KFTrayConfig{
		{
			Service:      "api",
			Namespace:    "default",
			Context:      "prod",
			WorkloadType: "service",
			Protocol:     "tcp",
			Alias:        "prod-api",
			LocalPort:    8080,
			RemotePort:   3000,
		},
	})
	output := filepath.Join(dir, "out.yaml")

	err := ConvertKFTrayToKPortal(input, output)
	require.NoError(t, err)

	raw, err := os.ReadFile(output)
	require.NoError(t, err)

	// Header present
	assert.True(t, strings.HasPrefix(string(raw), "# kportal configuration converted from kftray format"),
		"output must start with the header comment")

	// Parse the YAML body (strip comment lines for strict unmarshal)
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))

	require.Len(t, cfg.Contexts, 1)
	assert.Equal(t, "prod", cfg.Contexts[0].Name)
	require.Len(t, cfg.Contexts[0].Namespaces, 1)
	assert.Equal(t, "default", cfg.Contexts[0].Namespaces[0].Name)
	require.Len(t, cfg.Contexts[0].Namespaces[0].Forwards, 1)

	fwd := cfg.Contexts[0].Namespaces[0].Forwards[0]
	assert.Equal(t, "service/api", fwd.Resource)
	assert.Equal(t, "tcp", fwd.Protocol)
	assert.Equal(t, 3000, fwd.Port)
	assert.Equal(t, 8080, fwd.LocalPort)
	assert.Equal(t, "prod-api", fwd.Alias)
}

func TestConvertKFTrayToKPortal_MissingInputFile(t *testing.T) {
	dir := t.TempDir()
	err := ConvertKFTrayToKPortal(filepath.Join(dir, "nonexistent.json"), filepath.Join(dir, "out.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read input file")
}

func TestConvertKFTrayToKPortal_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(input, []byte("{not json}"), 0600))

	err := ConvertKFTrayToKPortal(input, filepath.Join(dir, "out.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse JSON")
}

func TestConvertKFTrayToKPortal_EmptyArray(t *testing.T) {
	dir := t.TempDir()
	input := writeJSON(t, dir, "empty.json", []KFTrayConfig{})
	output := filepath.Join(dir, "out.yaml")

	err := ConvertKFTrayToKPortal(input, output)
	require.NoError(t, err)

	raw, err := os.ReadFile(output)
	require.NoError(t, err)

	var cfg config.Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	assert.Empty(t, cfg.Contexts)
}

func TestConvertKFTrayToKPortal_UnwritableOutputDir(t *testing.T) {
	dir := t.TempDir()
	input := writeJSON(t, dir, "in.json", []KFTrayConfig{
		{Service: "svc", Namespace: "ns", Context: "ctx", WorkloadType: "service", Protocol: "tcp", LocalPort: 80, RemotePort: 80},
	})

	// Use a path that cannot be created (sub-dir of a non-existing dir)
	output := filepath.Join(dir, "no-such-subdir", "out.yaml")

	err := ConvertKFTrayToKPortal(input, output)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write output file")
}

func TestConvertKFTrayToKPortal_MultipleEntries_YAMLRoundtrip(t *testing.T) {
	dir := t.TempDir()
	entries := []KFTrayConfig{
		{Service: "postgres", Namespace: "db", Context: "prod", WorkloadType: "service", Protocol: "tcp", Alias: "pg", LocalPort: 5432, RemotePort: 5432},
		{Service: "redis", Namespace: "cache", Context: "prod", WorkloadType: "service", Protocol: "tcp", Alias: "rd", LocalPort: 6379, RemotePort: 6379},
		{Service: "api", Namespace: "default", Context: "staging", WorkloadType: "pod", Protocol: "tcp", LocalPort: 8080, RemotePort: 8080},
	}
	input := writeJSON(t, dir, "in.json", entries)
	output := filepath.Join(dir, "out.yaml")

	require.NoError(t, ConvertKFTrayToKPortal(input, output))

	raw, err := os.ReadFile(output)
	require.NoError(t, err)

	var cfg config.Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))

	// Two distinct contexts: prod, staging (sorted)
	require.Len(t, cfg.Contexts, 2)
	assert.Equal(t, "prod", cfg.Contexts[0].Name)
	assert.Equal(t, "staging", cfg.Contexts[1].Name)

	// prod has two namespaces sorted: cache, db
	prodNS := cfg.Contexts[0].Namespaces
	require.Len(t, prodNS, 2)
	assert.Equal(t, "cache", prodNS[0].Name)
	assert.Equal(t, "db", prodNS[1].Name)

	// staging/default has pod workload type
	stagingFwd := cfg.Contexts[1].Namespaces[0].Forwards[0]
	assert.Equal(t, "pod/api", stagingFwd.Resource)
}

func TestConvertKFTrayToKPortal_OutputFilePermissions(t *testing.T) {
	dir := t.TempDir()
	input := writeJSON(t, dir, "in.json", []KFTrayConfig{
		{Service: "svc", Namespace: "ns", Context: "ctx", WorkloadType: "service", Protocol: "tcp", LocalPort: 80, RemotePort: 80},
	})
	output := filepath.Join(dir, "out.yaml")

	require.NoError(t, ConvertKFTrayToKPortal(input, output))

	info, err := os.Stat(output)
	require.NoError(t, err)
	// Written with 0600 — owner rw, no group/other
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

// ─── GetConversionSummary ────────────────────────────────────────────────────

func TestGetConversionSummary_HappyPath(t *testing.T) {
	dir := t.TempDir()
	entries := []KFTrayConfig{
		{Service: "api", Namespace: "default", Context: "prod", WorkloadType: "service", Protocol: "tcp", LocalPort: 8080, RemotePort: 8080},
		{Service: "pg", Namespace: "db", Context: "prod", WorkloadType: "service", Protocol: "tcp", LocalPort: 5432, RemotePort: 5432},
		{Service: "api", Namespace: "default", Context: "staging", WorkloadType: "service", Protocol: "tcp", LocalPort: 8080, RemotePort: 8080},
	}
	input := writeJSON(t, dir, "in.json", entries)

	contextMap, total, err := GetConversionSummary(input)
	require.NoError(t, err)

	assert.Equal(t, 3, total)
	assert.Len(t, contextMap, 2)

	// prod context: 2 entries across 2 namespaces
	assert.Equal(t, 1, contextMap["prod"]["default"])
	assert.Equal(t, 1, contextMap["prod"]["db"])

	// staging context: 1 entry in default namespace
	assert.Equal(t, 1, contextMap["staging"]["default"])
}

func TestGetConversionSummary_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, _, err := GetConversionSummary(filepath.Join(dir, "ghost.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read input file")
}

func TestGetConversionSummary_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("not-json"), 0600))

	_, _, err := GetConversionSummary(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse JSON")
}

func TestGetConversionSummary_EmptyArray(t *testing.T) {
	dir := t.TempDir()
	input := writeJSON(t, dir, "empty.json", []KFTrayConfig{})

	contextMap, total, err := GetConversionSummary(input)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, contextMap)
}

func TestGetConversionSummary_SameNamespaceDifferentContexts(t *testing.T) {
	dir := t.TempDir()
	entries := []KFTrayConfig{
		{Service: "svc", Namespace: "default", Context: "ctx-a", LocalPort: 80, RemotePort: 80},
		{Service: "svc", Namespace: "default", Context: "ctx-a", LocalPort: 81, RemotePort: 81},
		{Service: "svc", Namespace: "default", Context: "ctx-b", LocalPort: 80, RemotePort: 80},
	}
	input := writeJSON(t, dir, "in.json", entries)

	contextMap, total, err := GetConversionSummary(input)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	// ctx-a/default has 2 services
	assert.Equal(t, 2, contextMap["ctx-a"]["default"])
	// ctx-b/default has 1 service
	assert.Equal(t, 1, contextMap["ctx-b"]["default"])
}

// ─── convertToKPortal edge cases ─────────────────────────────────────────────

func TestConvertToKPortal_EmptyInput(t *testing.T) {
	result := convertToKPortal([]KFTrayConfig{})
	assert.Empty(t, result.Contexts)
}

func TestConvertToKPortal_ZeroPorts(t *testing.T) {
	result := convertToKPortal([]KFTrayConfig{
		{Service: "svc", Namespace: "ns", Context: "ctx", WorkloadType: "service", Protocol: "tcp"},
	})
	require.Len(t, result.Contexts, 1)
	fwd := result.Contexts[0].Namespaces[0].Forwards[0]
	assert.Equal(t, 0, fwd.Port)
	assert.Equal(t, 0, fwd.LocalPort)
}

func TestConvertToKPortal_EmptyWorkloadType(t *testing.T) {
	// WorkloadType="" → resource becomes "/svc"
	result := convertToKPortal([]KFTrayConfig{
		{Service: "svc", Namespace: "ns", Context: "ctx", WorkloadType: "", Protocol: "tcp", LocalPort: 80, RemotePort: 80},
	})
	fwd := result.Contexts[0].Namespaces[0].Forwards[0]
	assert.Equal(t, "/svc", fwd.Resource)
}

func TestConvertToKPortal_ForwardsSortedByLocalPort(t *testing.T) {
	// Supply in reverse order; expect ascending local port after conversion
	cfgs := []KFTrayConfig{
		{Service: "c", Namespace: "ns", Context: "ctx", WorkloadType: "service", Protocol: "tcp", LocalPort: 9000, RemotePort: 9000},
		{Service: "a", Namespace: "ns", Context: "ctx", WorkloadType: "service", Protocol: "tcp", LocalPort: 1000, RemotePort: 1000},
		{Service: "b", Namespace: "ns", Context: "ctx", WorkloadType: "service", Protocol: "tcp", LocalPort: 5000, RemotePort: 5000},
	}
	result := convertToKPortal(cfgs)
	forwards := result.Contexts[0].Namespaces[0].Forwards
	require.Len(t, forwards, 3)
	assert.Equal(t, 1000, forwards[0].LocalPort)
	assert.Equal(t, 5000, forwards[1].LocalPort)
	assert.Equal(t, 9000, forwards[2].LocalPort)
}

func TestConvertToKPortal_ContextsAndNamespacesSortedAlphabetically(t *testing.T) {
	cfgs := []KFTrayConfig{
		{Service: "svc", Namespace: "z-ns", Context: "z-ctx", WorkloadType: "service", Protocol: "tcp", LocalPort: 80, RemotePort: 80},
		{Service: "svc", Namespace: "a-ns", Context: "z-ctx", WorkloadType: "service", Protocol: "tcp", LocalPort: 81, RemotePort: 81},
		{Service: "svc", Namespace: "m-ns", Context: "a-ctx", WorkloadType: "service", Protocol: "tcp", LocalPort: 82, RemotePort: 82},
	}
	result := convertToKPortal(cfgs)

	require.Len(t, result.Contexts, 2)
	assert.Equal(t, "a-ctx", result.Contexts[0].Name)
	assert.Equal(t, "z-ctx", result.Contexts[1].Name)

	zCtxNS := result.Contexts[1].Namespaces
	require.Len(t, zCtxNS, 2)
	assert.Equal(t, "a-ns", zCtxNS[0].Name)
	assert.Equal(t, "z-ns", zCtxNS[1].Name)
}

func TestConvertToKPortal_AliasPreservedWhenSet(t *testing.T) {
	cfgs := []KFTrayConfig{
		{Service: "svc", Namespace: "ns", Context: "ctx", WorkloadType: "service", Protocol: "tcp", Alias: "my-alias", LocalPort: 80, RemotePort: 80},
	}
	result := convertToKPortal(cfgs)
	assert.Equal(t, "my-alias", result.Contexts[0].Namespaces[0].Forwards[0].Alias)
}

func TestConvertToKPortal_DifferentProtocols(t *testing.T) {
	tests := []struct {
		protocol string
	}{
		{"tcp"},
		{"udp"},
		{""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run("protocol="+tt.protocol, func(t *testing.T) {
			result := convertToKPortal([]KFTrayConfig{
				{Service: "svc", Namespace: "ns", Context: "ctx", WorkloadType: "service", Protocol: tt.protocol, LocalPort: 80, RemotePort: 80},
			})
			assert.Equal(t, tt.protocol, result.Contexts[0].Namespaces[0].Forwards[0].Protocol)
		})
	}
}
