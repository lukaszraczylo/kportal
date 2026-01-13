package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseDurationOrDefault tests the duration parsing helper
func TestParseDurationOrDefault(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		defaultDur time.Duration
		expected   time.Duration
	}{
		{"empty string returns default", "", 5 * time.Second, 5 * time.Second},
		{"valid duration seconds", "3s", 5 * time.Second, 3 * time.Second},
		{"valid duration minutes", "25m", 5 * time.Second, 25 * time.Minute},
		{"valid duration hours", "1h", 5 * time.Second, 1 * time.Hour},
		{"valid duration milliseconds", "100ms", 5 * time.Second, 100 * time.Millisecond},
		{"invalid duration returns default", "invalid", 5 * time.Second, 5 * time.Second},
		{"missing unit returns default", "30", 5 * time.Second, 5 * time.Second},
		{"negative duration", "-5s", 5 * time.Second, -5 * time.Second}, // time.ParseDuration accepts negative
		{"complex duration", "1h30m", 5 * time.Second, 1*time.Hour + 30*time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDurationOrDefault(tt.value, tt.defaultDur)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfig_GetHealthCheckIntervalOrDefault tests health check interval getter
func TestConfig_GetHealthCheckIntervalOrDefault(t *testing.T) {
	tests := []struct {
		config   *Config
		name     string
		expected time.Duration
	}{
		{
			name:     "nil health check returns default",
			config:   &Config{},
			expected: DefaultHealthCheckInterval,
		},
		{
			name: "empty interval returns default",
			config: &Config{
				HealthCheck: &HealthCheckSpec{},
			},
			expected: DefaultHealthCheckInterval,
		},
		{
			name: "valid interval",
			config: &Config{
				HealthCheck: &HealthCheckSpec{Interval: "5s"},
			},
			expected: 5 * time.Second,
		},
		{
			name: "invalid interval returns default",
			config: &Config{
				HealthCheck: &HealthCheckSpec{Interval: "invalid"},
			},
			expected: DefaultHealthCheckInterval,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetHealthCheckIntervalOrDefault()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfig_GetHealthCheckTimeoutOrDefault tests health check timeout getter
func TestConfig_GetHealthCheckTimeoutOrDefault(t *testing.T) {
	tests := []struct {
		config   *Config
		name     string
		expected time.Duration
	}{
		{
			name:     "nil health check returns default",
			config:   &Config{},
			expected: DefaultHealthCheckTimeout,
		},
		{
			name: "empty timeout returns default",
			config: &Config{
				HealthCheck: &HealthCheckSpec{},
			},
			expected: DefaultHealthCheckTimeout,
		},
		{
			name: "valid timeout",
			config: &Config{
				HealthCheck: &HealthCheckSpec{Timeout: "1s"},
			},
			expected: 1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetHealthCheckTimeoutOrDefault()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfig_GetHealthCheckMethod tests health check method getter
func TestConfig_GetHealthCheckMethod(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected string
	}{
		{
			name:     "nil health check returns default",
			config:   &Config{},
			expected: DefaultHealthCheckMethod,
		},
		{
			name: "empty method returns default",
			config: &Config{
				HealthCheck: &HealthCheckSpec{},
			},
			expected: DefaultHealthCheckMethod,
		},
		{
			name: "tcp-dial method",
			config: &Config{
				HealthCheck: &HealthCheckSpec{Method: "tcp-dial"},
			},
			expected: "tcp-dial",
		},
		{
			name: "data-transfer method",
			config: &Config{
				HealthCheck: &HealthCheckSpec{Method: "data-transfer"},
			},
			expected: "data-transfer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetHealthCheckMethod()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfig_GetMaxConnectionAge tests max connection age getter
func TestConfig_GetMaxConnectionAge(t *testing.T) {
	tests := []struct {
		config   *Config
		name     string
		expected time.Duration
	}{
		{
			name:     "nil health check returns default",
			config:   &Config{},
			expected: DefaultMaxConnectionAge,
		},
		{
			name: "empty max age returns default",
			config: &Config{
				HealthCheck: &HealthCheckSpec{},
			},
			expected: DefaultMaxConnectionAge,
		},
		{
			name: "valid max age",
			config: &Config{
				HealthCheck: &HealthCheckSpec{MaxConnectionAge: "20m"},
			},
			expected: 20 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetMaxConnectionAge()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfig_GetMaxIdleTime tests max idle time getter
func TestConfig_GetMaxIdleTime(t *testing.T) {
	tests := []struct {
		config   *Config
		name     string
		expected time.Duration
	}{
		{
			name:     "nil health check returns default",
			config:   &Config{},
			expected: DefaultMaxIdleTime,
		},
		{
			name: "empty max idle returns default",
			config: &Config{
				HealthCheck: &HealthCheckSpec{},
			},
			expected: DefaultMaxIdleTime,
		},
		{
			name: "valid max idle",
			config: &Config{
				HealthCheck: &HealthCheckSpec{MaxIdleTime: "5m"},
			},
			expected: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetMaxIdleTime()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfig_GetTCPKeepalive tests TCP keepalive getter
func TestConfig_GetTCPKeepalive(t *testing.T) {
	tests := []struct {
		config   *Config
		name     string
		expected time.Duration
	}{
		{
			name:     "nil reliability returns default",
			config:   &Config{},
			expected: DefaultTCPKeepalive,
		},
		{
			name: "empty keepalive returns default",
			config: &Config{
				Reliability: &ReliabilitySpec{},
			},
			expected: DefaultTCPKeepalive,
		},
		{
			name: "valid keepalive",
			config: &Config{
				Reliability: &ReliabilitySpec{TCPKeepalive: "15s"},
			},
			expected: 15 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetTCPKeepalive()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfig_GetRetryOnStale tests retry on stale getter
func TestConfig_GetRetryOnStale(t *testing.T) {
	tests := []struct {
		config   *Config
		name     string
		expected bool
	}{
		{
			name:     "nil reliability returns default true",
			config:   &Config{},
			expected: true,
		},
		{
			name: "explicit false",
			config: &Config{
				Reliability: &ReliabilitySpec{RetryOnStale: false},
			},
			expected: false,
		},
		{
			name: "explicit true",
			config: &Config{
				Reliability: &ReliabilitySpec{RetryOnStale: true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetRetryOnStale()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfig_GetWatchdogPeriod tests watchdog period getter
func TestConfig_GetWatchdogPeriod(t *testing.T) {
	tests := []struct {
		config   *Config
		name     string
		expected time.Duration
	}{
		{
			name:     "nil reliability returns default",
			config:   &Config{},
			expected: DefaultWatchdogPeriod,
		},
		{
			name: "empty period returns default",
			config: &Config{
				Reliability: &ReliabilitySpec{},
			},
			expected: DefaultWatchdogPeriod,
		},
		{
			name: "valid period",
			config: &Config{
				Reliability: &ReliabilitySpec{WatchdogPeriod: "1m"},
			},
			expected: 1 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetWatchdogPeriod()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfig_GetDialTimeout tests dial timeout getter
func TestConfig_GetDialTimeout(t *testing.T) {
	tests := []struct {
		config   *Config
		name     string
		expected time.Duration
	}{
		{
			name:     "nil reliability returns default",
			config:   &Config{},
			expected: DefaultDialTimeout,
		},
		{
			name: "empty timeout returns default",
			config: &Config{
				Reliability: &ReliabilitySpec{},
			},
			expected: DefaultDialTimeout,
		},
		{
			name: "valid timeout",
			config: &Config{
				Reliability: &ReliabilitySpec{DialTimeout: "10s"},
			},
			expected: 10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetDialTimeout()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfig_IsMDNSEnabled tests mDNS enabled getter
func TestConfig_IsMDNSEnabled(t *testing.T) {
	tests := []struct {
		config   *Config
		name     string
		expected bool
	}{
		{
			name:     "nil MDNS returns false",
			config:   &Config{},
			expected: false,
		},
		{
			name: "MDNS disabled",
			config: &Config{
				MDNS: &MDNSSpec{Enabled: false},
			},
			expected: false,
		},
		{
			name: "MDNS enabled",
			config: &Config{
				MDNS: &MDNSSpec{Enabled: true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsMDNSEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestForward_IsHTTPLogEnabled tests HTTP log enabled check
func TestForward_IsHTTPLogEnabled(t *testing.T) {
	tests := []struct {
		name     string
		forward  Forward
		expected bool
	}{
		{
			name:     "nil HTTPLog",
			forward:  Forward{Resource: "pod/app", Port: 8080, LocalPort: 8080},
			expected: false,
		},
		{
			name: "HTTPLog disabled",
			forward: Forward{
				Resource:  "pod/app",
				Port:      8080,
				LocalPort: 8080,
				HTTPLog:   &HTTPLogSpec{Enabled: false},
			},
			expected: false,
		},
		{
			name: "HTTPLog enabled",
			forward: Forward{
				Resource:  "pod/app",
				Port:      8080,
				LocalPort: 8080,
				HTTPLog:   &HTTPLogSpec{Enabled: true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.forward.IsHTTPLogEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestForward_GetHTTPLogMaxBodySize tests HTTP log max body size
func TestForward_GetHTTPLogMaxBodySize(t *testing.T) {
	tests := []struct {
		name     string
		forward  Forward
		expected int
	}{
		{
			name:     "nil HTTPLog returns default",
			forward:  Forward{Resource: "pod/app", Port: 8080, LocalPort: 8080},
			expected: DefaultHTTPLogMaxBodySize,
		},
		{
			name: "zero max body size returns default",
			forward: Forward{
				Resource:  "pod/app",
				Port:      8080,
				LocalPort: 8080,
				HTTPLog:   &HTTPLogSpec{MaxBodySize: 0},
			},
			expected: DefaultHTTPLogMaxBodySize,
		},
		{
			name: "negative max body size returns default",
			forward: Forward{
				Resource:  "pod/app",
				Port:      8080,
				LocalPort: 8080,
				HTTPLog:   &HTTPLogSpec{MaxBodySize: -100},
			},
			expected: DefaultHTTPLogMaxBodySize,
		},
		{
			name: "custom max body size",
			forward: Forward{
				Resource:  "pod/app",
				Port:      8080,
				LocalPort: 8080,
				HTTPLog:   &HTTPLogSpec{MaxBodySize: 2048},
			},
			expected: 2048,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.forward.GetHTTPLogMaxBodySize()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestForward_GetMDNSAlias tests mDNS alias generation
func TestForward_GetMDNSAlias(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		forward  Forward
	}{
		{
			name: "explicit alias",
			forward: Forward{
				Resource:  "pod/my-app",
				Port:      8080,
				LocalPort: 8080,
				Alias:     "my-custom-alias",
			},
			expected: "my-custom-alias",
		},
		{
			name: "pod with name - extracts name",
			forward: Forward{
				Resource:  "pod/my-app",
				Port:      8080,
				LocalPort: 8080,
			},
			expected: "my-app",
		},
		{
			name: "service with name - extracts name",
			forward: Forward{
				Resource:  "service/postgres",
				Port:      5432,
				LocalPort: 5432,
			},
			expected: "postgres",
		},
		{
			name: "pod without name (selector-based) - returns empty",
			forward: Forward{
				Resource:  "pod",
				Selector:  "app=nginx",
				Port:      80,
				LocalPort: 8080,
			},
			expected: "",
		},
		{
			name: "empty resource - returns empty",
			forward: Forward{
				Resource:  "",
				Port:      8080,
				LocalPort: 8080,
			},
			expected: "",
		},
		{
			name: "resource with empty name after slash",
			forward: Forward{
				Resource:  "pod/",
				Port:      8080,
				LocalPort: 8080,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.forward.GetMDNSAlias()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestLoadConfig_FileTooLarge tests file size limit
func TestLoadConfig_FileTooLarge(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create a file larger than maxConfigSize (10MB)
	// We'll use a smaller buffer to avoid memory issues
	// Just verify the check happens by creating a file slightly over 10MB
	largeData := make([]byte, 10*1024*1024+1) // 10MB + 1 byte
	for i := range largeData {
		largeData[i] = 'a'
	}

	err := os.WriteFile(configPath, largeData, 0600)
	require.NoError(t, err)

	cfg, err := LoadConfig(configPath)
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "config file too large")
}

// TestLoadConfig_WithHealthCheckAndReliability tests parsing with all config sections
func TestLoadConfig_WithHealthCheckAndReliability(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	yaml := `contexts:
  - name: dev
    namespaces:
      - name: default
        forwards:
          - resource: pod/app
            port: 8080
            localPort: 8080
healthCheck:
  interval: "5s"
  timeout: "1s"
  method: "tcp-dial"
  maxConnectionAge: "20m"
  maxIdleTime: "5m"
reliability:
  tcpKeepalive: "15s"
  dialTimeout: "10s"
  retryOnStale: true
  watchdogPeriod: "1m"
mdns:
  enabled: true
`

	err := os.WriteFile(configPath, []byte(yaml), 0600)
	require.NoError(t, err)

	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify health check settings
	assert.Equal(t, 5*time.Second, cfg.GetHealthCheckIntervalOrDefault())
	assert.Equal(t, 1*time.Second, cfg.GetHealthCheckTimeoutOrDefault())
	assert.Equal(t, "tcp-dial", cfg.GetHealthCheckMethod())
	assert.Equal(t, 20*time.Minute, cfg.GetMaxConnectionAge())
	assert.Equal(t, 5*time.Minute, cfg.GetMaxIdleTime())

	// Verify reliability settings
	assert.Equal(t, 15*time.Second, cfg.GetTCPKeepalive())
	assert.Equal(t, 10*time.Second, cfg.GetDialTimeout())
	assert.True(t, cfg.GetRetryOnStale())
	assert.Equal(t, 1*time.Minute, cfg.GetWatchdogPeriod())

	// Verify mDNS
	assert.True(t, cfg.IsMDNSEnabled())
}

// TestParseConfig_RejectsUnknownKeys tests strict parsing
func TestParseConfig_RejectsUnknownKeys(t *testing.T) {
	yaml := `contexts:
  - name: dev
    namespaces:
      - name: default
        forwards:
          - resource: pod/app
            port: 8080
            localPort: 8080
unknownKey: value
`

	cfg, err := ParseConfig([]byte(yaml))
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "failed to parse YAML")
}

// TestHTTPLogSpec_FullStruct tests full HTTPLogSpec parsing
func TestHTTPLogSpec_FullStruct(t *testing.T) {
	yaml := `contexts:
  - name: dev
    namespaces:
      - name: default
        forwards:
          - resource: service/api
            port: 8080
            localPort: 8080
            httpLog:
              enabled: true
              logFile: "/tmp/http.log"
              maxBodySize: 2048
              includeHeaders: true
              filterPath: "/api/*"
`

	cfg, err := ParseConfig([]byte(yaml))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	fwd := cfg.Contexts[0].Namespaces[0].Forwards[0]
	require.NotNil(t, fwd.HTTPLog)
	assert.True(t, fwd.HTTPLog.Enabled)
	assert.Equal(t, "/tmp/http.log", fwd.HTTPLog.LogFile)
	assert.Equal(t, 2048, fwd.HTTPLog.MaxBodySize)
	assert.True(t, fwd.HTTPLog.IncludeHeaders)
	assert.Equal(t, "/api/*", fwd.HTTPLog.FilterPath)
}
