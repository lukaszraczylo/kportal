package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeContextSelection(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{name: "nil input", input: nil, want: nil},
		{name: "single name", input: []string{"team-a"}, want: []string{"team-a"}},
		{name: "repeated flag", input: []string{"team-a", "team-b"}, want: []string{"team-a", "team-b"}},
		{name: "comma separated", input: []string{"team-a,team-b"}, want: []string{"team-a", "team-b"}},
		{name: "repeated and comma mixed", input: []string{"team-a,team-b", "team-c"}, want: []string{"team-a", "team-b", "team-c"}},
		{name: "whitespace trimmed", input: []string{" team-a , team-b "}, want: []string{"team-a", "team-b"}},
		{name: "empty segments dropped", input: []string{"team-a,,team-b"}, want: []string{"team-a", "team-b"}},
		{name: "duplicates removed", input: []string{"team-a,team-a", "team-a"}, want: []string{"team-a"}},
		{name: "only separators", input: []string{",,"}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeContextSelection(tt.input))
		})
	}
}

func TestPortsCanConflict(t *testing.T) {
	tests := []struct {
		name     string
		contextA string
		contextB string
		active   []string
		want     bool
	}{
		{
			name:     "same context always conflicts",
			contextA: "team-a", contextB: "team-a", active: []string{"team-b"},
			want: true,
		},
		{
			name:     "no selection means everything runs together",
			contextA: "team-a", contextB: "team-b", active: nil,
			want: true,
		},
		{
			name:     "both selected conflict",
			contextA: "team-a", contextB: "team-b", active: []string{"team-a", "team-b"},
			want: true,
		},
		{
			name:     "only one selected does not conflict",
			contextA: "team-a", contextB: "team-b", active: []string{"team-a"},
			want: false,
		},
		{
			name:     "neither selected does not conflict",
			contextA: "team-a", contextB: "team-b", active: []string{"team-c"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, PortsCanConflict(tt.contextA, tt.contextB, tt.active))
		})
	}
}

// threeContextConfig builds a config with three contexts, each holding one
// forward on the same local port - the layout this feature exists to enable.
func threeContextConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(`
mdns:
  enabled: false
healthCheck:
  interval: "5s"
contexts:
  - name: team-a
    namespaces:
      - name: rate-service
        forwards:
          - resource: pod/rpc-server
            protocol: tcp
            port: 50051
            localPort: 3004
            alias: rate-service-a
  - name: team-b
    namespaces:
      - name: rate-service
        forwards:
          - resource: pod/rpc-server
            protocol: tcp
            port: 50051
            localPort: 3004
            alias: rate-service-b
  - name: team-c
    namespaces:
      - name: rate-service
        forwards:
          - resource: pod/rpc-server
            protocol: tcp
            port: 50051
            localPort: 3004
            alias: rate-service-c
`))
	require.NoError(t, err)
	return cfg
}

func contextNames(cfg *Config) []string {
	names := make([]string, 0, len(cfg.Contexts))
	for _, ctx := range cfg.Contexts {
		names = append(names, ctx.Name)
	}
	return names
}

func TestConfig_SelectContexts(t *testing.T) {
	tests := []struct {
		name      string
		wantErr   string
		selection []string
		want      []string
	}{
		{name: "empty selection keeps everything", selection: nil, want: []string{"team-a", "team-b", "team-c"}},
		{name: "single context", selection: []string{"team-b"}, want: []string{"team-b"}},
		{name: "multiple contexts", selection: []string{"team-a", "team-c"}, want: []string{"team-a", "team-c"}},
		{
			name:      "config order wins over flag order",
			selection: []string{"team-c", "team-a"},
			want:      []string{"team-a", "team-c"},
		},
		{
			name:      "unknown context errors and lists what exists",
			selection: []string{"team-z"},
			wantErr:   `unknown context "team-z" in --context (available: "team-a", "team-b", "team-c")`,
		},
		{
			name:      "one unknown among known still errors",
			selection: []string{"team-a", "team-z"},
			wantErr:   `unknown context "team-z"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := threeContextConfig(t)

			got, err := cfg.SelectContexts(tt.selection)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, contextNames(got))
			// The original config must not be modified.
			assert.Equal(t, []string{"team-a", "team-b", "team-c"}, contextNames(cfg))
		})
	}
}

func TestConfig_SelectContexts_PreservesTopLevelSettings(t *testing.T) {
	cfg := threeContextConfig(t)

	selected, err := cfg.SelectContexts([]string{"team-a"})
	require.NoError(t, err)

	require.NotNil(t, selected.HealthCheck)
	assert.Equal(t, "5s", selected.HealthCheck.Interval)
	assert.Equal(t, cfg.MDNS, selected.MDNS)
	assert.Len(t, selected.GetAllForwards(), 1)
}

func TestConfig_SelectContexts_EmptyConfig(t *testing.T) {
	cfg := &Config{}

	// An empty selection is a no-op even with nothing to select.
	same, err := cfg.SelectContexts(nil)
	require.NoError(t, err)
	assert.Same(t, cfg, same)

	_, err = cfg.SelectContexts([]string{"team-a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "none defined in the config file")
}

func TestConfig_SelectContexts_ContextWithNoForwards(t *testing.T) {
	cfg := &Config{Contexts: []Context{{Name: "empty"}}}

	selected, err := cfg.SelectContexts([]string{"empty"})
	require.NoError(t, err)
	assert.Equal(t, []string{"empty"}, contextNames(selected))
	assert.Empty(t, selected.GetAllForwards())
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".kportal.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0600))
	return path
}

// sharedPortConfig is the reporter's layout from issue #80: the same service on
// the same local port in every cluster.
const sharedPortConfig = `contexts:
  - name: team-a
    namespaces:
      - name: rate-service
        forwards:
          - resource: pod/rpc-server
            protocol: tcp
            port: 50051
            localPort: 3004
  - name: team-b
    namespaces:
      - name: rate-service
        forwards:
          - resource: pod/rpc-server
            protocol: tcp
            port: 50051
            localPort: 3004
`

func TestLoadForRuntime_SharedPortsAcrossNonSelectedContexts(t *testing.T) {
	path := writeConfig(t, sharedPortConfig)

	// Without a selection both contexts run together, so the shared port is a
	// genuine conflict.
	_, err := LoadForRuntime(path, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Duplicate local port 3004")

	// Selecting one context makes the layout legal and narrows what runs.
	cfg, err := LoadForRuntime(path, []string{"team-a"})
	require.NoError(t, err)
	assert.Equal(t, []string{"team-a"}, contextNames(cfg))
	require.Len(t, cfg.GetAllForwards(), 1)

	// Selecting both brings the conflict back - they really would collide.
	_, err = LoadForRuntime(path, []string{"team-a", "team-b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Duplicate local port 3004")
}

func TestLoadForRuntime_UnknownSelectedContextIsDropped(t *testing.T) {
	path := writeConfig(t, sharedPortConfig)

	// A context renamed in the file must not freeze hot-reload for the session.
	cfg, err := LoadForRuntime(path, []string{"team-a", "renamed-away"})
	require.NoError(t, err)
	assert.Equal(t, []string{"team-a"}, contextNames(cfg))
}

func TestLoadForRuntime_SelectionReducingToNothingIsAnError(t *testing.T) {
	path := writeConfig(t, sharedPortConfig)

	cfg, err := LoadForRuntime(path, []string{"gone"})
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), `"gone"`)
	assert.Contains(t, err.Error(), `"team-a"`)
}

func TestLoadForRuntime_ReportsErrorsInNonSelectedContexts(t *testing.T) {
	// A broken forward in a context we are not forwarding is still a broken
	// config file, and must be reported.
	path := writeConfig(t, `contexts:
  - name: team-a
    namespaces:
      - name: default
        forwards:
          - resource: pod/ok
            protocol: tcp
            port: 80
            localPort: 8080
  - name: team-b
    namespaces:
      - name: default
        forwards:
          - resource: pod/bad
            protocol: tcp
            port: 80
            localPort: 999999
`)

	_, err := LoadForRuntime(path, []string{"team-a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "localPort")
}

func TestLoadForRuntime_MissingFile(t *testing.T) {
	_, err := LoadForRuntime(filepath.Join(t.TempDir(), "absent.yaml"), nil)
	assert.ErrorIs(t, err, ErrConfigNotFound)
}
