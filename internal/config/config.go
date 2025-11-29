package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ErrConfigNotFound is returned when the configuration file does not exist
var ErrConfigNotFound = fmt.Errorf("config file not found")

const (
	// maxConfigSize is the maximum allowed configuration file size (10MB)
	maxConfigSize = 10 * 1024 * 1024

	// Default health check settings
	DefaultHealthCheckInterval = 3 * time.Second  // How often to check connection health
	DefaultHealthCheckTimeout  = 2 * time.Second  // Timeout for health check probes
	DefaultHealthCheckMethod   = "data-transfer"  // More reliable than tcp-dial
	DefaultMaxConnectionAge    = 25 * time.Minute // Reconnect before k8s 30min timeout
	DefaultMaxIdleTime         = 10 * time.Minute // Reconnect if no activity

	// Default reliability settings
	DefaultTCPKeepalive   = 30 * time.Second // OS-level TCP keepalive interval
	DefaultDialTimeout    = 30 * time.Second // Connection establishment timeout
	DefaultWatchdogPeriod = 30 * time.Second // Goroutine health check interval

	// Default HTTP logging settings
	DefaultHTTPLogMaxBodySize = 1024 * 1024 // 1MB max body size for logging
)

// Config represents the root configuration structure from .kportal.yaml
type Config struct {
	Contexts    []Context        `yaml:"contexts"`
	HealthCheck *HealthCheckSpec `yaml:"healthCheck,omitempty"`
	Reliability *ReliabilitySpec `yaml:"reliability,omitempty"`
	MDNS        *MDNSSpec        `yaml:"mdns,omitempty"`
}

// MDNSSpec configures mDNS (multicast DNS) hostname publishing
// When enabled, forwards with aliases can be accessed via <alias>.local hostnames
type MDNSSpec struct {
	Enabled bool `yaml:"enabled"` // Enable mDNS hostname publishing
}

// HealthCheckSpec configures health check behavior
type HealthCheckSpec struct {
	Interval         string `yaml:"interval,omitempty"`         // e.g., "3s", "5s"
	Timeout          string `yaml:"timeout,omitempty"`          // e.g., "2s"
	Method           string `yaml:"method,omitempty"`           // "tcp-dial" | "data-transfer"
	MaxConnectionAge string `yaml:"maxConnectionAge,omitempty"` // e.g., "25m" - reconnect before k8s timeout
	MaxIdleTime      string `yaml:"maxIdleTime,omitempty"`      // e.g., "10m" - reconnect if no activity
}

// ReliabilitySpec configures connection reliability features
type ReliabilitySpec struct {
	TCPKeepalive   string `yaml:"tcpKeepalive,omitempty"`   // e.g., "30s" - OS-level keepalive
	DialTimeout    string `yaml:"dialTimeout,omitempty"`    // e.g., "30s" - connection dial timeout
	RetryOnStale   bool   `yaml:"retryOnStale,omitempty"`   // Auto-reconnect on stale detection
	WatchdogPeriod string `yaml:"watchdogPeriod,omitempty"` // e.g., "30s" - goroutine watchdog interval
}

// parseDurationOrDefault parses a duration string and returns the default if empty or invalid.
func parseDurationOrDefault(value string, defaultDur time.Duration) time.Duration {
	if value == "" {
		return defaultDur
	}
	if d, err := time.ParseDuration(value); err == nil {
		return d
	}
	return defaultDur
}

// GetHealthCheckIntervalOrDefault returns the health check interval or default value
func (c *Config) GetHealthCheckIntervalOrDefault() time.Duration {
	if c.HealthCheck == nil {
		return DefaultHealthCheckInterval
	}
	return parseDurationOrDefault(c.HealthCheck.Interval, DefaultHealthCheckInterval)
}

// GetHealthCheckTimeoutOrDefault returns the health check timeout or default value
func (c *Config) GetHealthCheckTimeoutOrDefault() time.Duration {
	if c.HealthCheck == nil {
		return DefaultHealthCheckTimeout
	}
	return parseDurationOrDefault(c.HealthCheck.Timeout, DefaultHealthCheckTimeout)
}

// GetHealthCheckMethod returns the health check method or default
func (c *Config) GetHealthCheckMethod() string {
	if c.HealthCheck != nil && c.HealthCheck.Method != "" {
		return c.HealthCheck.Method
	}
	return DefaultHealthCheckMethod
}

// GetMaxConnectionAge returns the max connection age or default
func (c *Config) GetMaxConnectionAge() time.Duration {
	if c.HealthCheck == nil {
		return DefaultMaxConnectionAge
	}
	return parseDurationOrDefault(c.HealthCheck.MaxConnectionAge, DefaultMaxConnectionAge)
}

// GetMaxIdleTime returns the max idle time or default
func (c *Config) GetMaxIdleTime() time.Duration {
	if c.HealthCheck == nil {
		return DefaultMaxIdleTime
	}
	return parseDurationOrDefault(c.HealthCheck.MaxIdleTime, DefaultMaxIdleTime)
}

// GetTCPKeepalive returns the TCP keepalive duration or default
func (c *Config) GetTCPKeepalive() time.Duration {
	if c.Reliability == nil {
		return DefaultTCPKeepalive
	}
	return parseDurationOrDefault(c.Reliability.TCPKeepalive, DefaultTCPKeepalive)
}

// GetRetryOnStale returns whether to retry on stale connections
func (c *Config) GetRetryOnStale() bool {
	if c.Reliability != nil {
		return c.Reliability.RetryOnStale
	}
	return true // Default: enabled
}

// GetWatchdogPeriod returns the goroutine watchdog check period or default
func (c *Config) GetWatchdogPeriod() time.Duration {
	if c.Reliability == nil {
		return DefaultWatchdogPeriod
	}
	return parseDurationOrDefault(c.Reliability.WatchdogPeriod, DefaultWatchdogPeriod)
}

// GetDialTimeout returns the connection dial timeout or default
func (c *Config) GetDialTimeout() time.Duration {
	if c.Reliability == nil {
		return DefaultDialTimeout
	}
	return parseDurationOrDefault(c.Reliability.DialTimeout, DefaultDialTimeout)
}

// IsMDNSEnabled returns whether mDNS hostname publishing is enabled
func (c *Config) IsMDNSEnabled() bool {
	return c.MDNS != nil && c.MDNS.Enabled
}

// Context represents a Kubernetes context with its namespaces
type Context struct {
	Name       string      `yaml:"name"`
	Namespaces []Namespace `yaml:"namespaces"`
}

// Namespace represents a Kubernetes namespace with its forwards
type Namespace struct {
	Name     string    `yaml:"name"`
	Forwards []Forward `yaml:"forwards"`
}

// HTTPLogSpec configures HTTP traffic logging for a forward
type HTTPLogSpec struct {
	Enabled        bool   `yaml:"enabled"`                  // Enable HTTP logging
	LogFile        string `yaml:"logFile,omitempty"`        // Output file (empty = stdout)
	MaxBodySize    int    `yaml:"maxBodySize,omitempty"`    // Max body size to log (default 1MB)
	IncludeHeaders bool   `yaml:"includeHeaders,omitempty"` // Include headers in log
	FilterPath     string `yaml:"filterPath,omitempty"`     // Optional glob filter for paths
}

// UnmarshalYAML implements custom unmarshaling to support both bool and struct formats
// Allows: httpLog: true OR httpLog: { enabled: true, ... }
func (h *HTTPLogSpec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// First try to unmarshal as a boolean
	var boolVal bool
	if err := unmarshal(&boolVal); err == nil {
		h.Enabled = boolVal
		return nil
	}

	// Otherwise try to unmarshal as a struct
	type httpLogSpecAlias HTTPLogSpec // Use alias to avoid infinite recursion
	var spec httpLogSpecAlias
	if err := unmarshal(&spec); err != nil {
		return err
	}
	*h = HTTPLogSpec(spec)
	return nil
}

// Forward represents a single port-forward configuration
type Forward struct {
	Resource  string       `yaml:"resource"`          // e.g., "pod/my-app", "service/postgres", "pod"
	Selector  string       `yaml:"selector"`          // Label selector for pod resolution (e.g., "app=nginx,env=prod")
	Protocol  string       `yaml:"protocol"`          // tcp or udp
	Port      int          `yaml:"port"`              // Remote port
	LocalPort int          `yaml:"localPort"`         // Local port
	Alias     string       `yaml:"alias,omitempty"`   // Optional human-readable alias for this forward
	HTTPLog   *HTTPLogSpec `yaml:"httpLog,omitempty"` // Optional HTTP traffic logging

	// Runtime fields (not in YAML)
	contextName   string
	namespaceName string
}

// ID returns a unique identifier for this forward configuration.
// Format: alias:localPort (if alias provided) or context/namespace/resource:localPort
func (f *Forward) ID() string {
	if f.Alias != "" {
		return fmt.Sprintf("%s:%d", f.Alias, f.LocalPort)
	}
	return fmt.Sprintf("%s/%s/%s:%d", f.contextName, f.namespaceName, f.Resource, f.LocalPort)
}

// String returns a human-readable representation of the forward.
// Format: alias:port→localPort (if alias provided) or context/namespace/resource:port→localPort
func (f *Forward) String() string {
	if f.Alias != "" {
		return fmt.Sprintf("%s:%d→%d", f.Alias, f.Port, f.LocalPort)
	}
	if f.Selector != "" {
		return fmt.Sprintf("%s/%s/%s[%s]:%d→%d",
			f.contextName, f.namespaceName, f.Resource, f.Selector, f.Port, f.LocalPort)
	}
	return fmt.Sprintf("%s/%s/%s:%d→%d",
		f.contextName, f.namespaceName, f.Resource, f.Port, f.LocalPort)
}

// SetContext sets the context and namespace names for this forward.
// This is used during config parsing to populate runtime fields.
func (f *Forward) SetContext(ctx, ns string) {
	f.contextName = ctx
	f.namespaceName = ns
}

// GetContext returns the context name for this forward.
func (f *Forward) GetContext() string {
	return f.contextName
}

// GetNamespace returns the namespace name for this forward.
func (f *Forward) GetNamespace() string {
	return f.namespaceName
}

// IsHTTPLogEnabled returns true if HTTP logging is enabled for this forward
func (f *Forward) IsHTTPLogEnabled() bool {
	return f.HTTPLog != nil && f.HTTPLog.Enabled
}

// GetHTTPLogMaxBodySize returns the max body size for HTTP logging
func (f *Forward) GetHTTPLogMaxBodySize() int {
	if f.HTTPLog == nil || f.HTTPLog.MaxBodySize <= 0 {
		return DefaultHTTPLogMaxBodySize
	}
	return f.HTTPLog.MaxBodySize
}

// GetMDNSAlias returns the alias to use for mDNS hostname registration.
// If an explicit alias is set, it returns that.
// Otherwise, it generates one from the resource name (e.g., "service/logto" -> "logto").
func (f *Forward) GetMDNSAlias() string {
	if f.Alias != "" {
		return f.Alias
	}

	// Generate alias from resource name
	// Format is "type/name" (e.g., "service/logto", "pod/my-app")
	parts := strings.SplitN(f.Resource, "/", 2)
	if len(parts) == 2 && parts[1] != "" {
		return parts[1]
	}

	// Fallback: can't generate a valid alias (e.g., "pod" with selector)
	return ""
}

// LoadConfig loads and parses the configuration file from the given path.
func LoadConfig(path string) (*Config, error) {
	// Validate file size before reading
	fileInfo, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrConfigNotFound
		}
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}

	if fileInfo.Size() > maxConfigSize {
		return nil, fmt.Errorf("config file too large: %d bytes (max %d)", fileInfo.Size(), maxConfigSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return ParseConfig(data)
}

// ParseConfig parses YAML configuration data into a Config struct.
// It uses strict parsing that rejects unknown keys to catch typos.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config

	// Use decoder with KnownFields to reject unknown keys (catches typos)
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Populate runtime fields (context and namespace names)
	for i := range cfg.Contexts {
		ctx := &cfg.Contexts[i]
		for j := range ctx.Namespaces {
			ns := &ctx.Namespaces[j]
			for k := range ns.Forwards {
				fwd := &ns.Forwards[k]
				fwd.SetContext(ctx.Name, ns.Name)
			}
		}
	}

	return &cfg, nil
}

// GetAllForwards returns a flat list of all forwards across all contexts and namespaces.
func (c *Config) GetAllForwards() []Forward {
	var forwards []Forward

	for _, ctx := range c.Contexts {
		for _, ns := range ctx.Namespaces {
			forwards = append(forwards, ns.Forwards...)
		}
	}

	return forwards
}

// NewEmptyConfig returns a minimal empty configuration with no forwards.
// This is used when creating a new config file for the first time.
func NewEmptyConfig() *Config {
	return &Config{
		Contexts: []Context{},
	}
}

// IsEmpty returns true if the configuration has no forwards defined.
func (c *Config) IsEmpty() bool {
	return len(c.Contexts) == 0 || len(c.GetAllForwards()) == 0
}

// CreateEmptyConfigFile creates a new empty configuration file at the given path.
// Returns an error if the file already exists or cannot be created.
func CreateEmptyConfigFile(path string) error {
	// Check if file already exists
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config file already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check config file: %w", err)
	}

	cfg := NewEmptyConfig()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal empty config: %w", err)
	}

	// Add a helpful comment header
	header := `# kportal configuration file
# Add port forwards using the 'n' key in the TUI, or manually add them below.
#
# Example forward:
#   contexts:
#     - name: my-cluster
#       namespaces:
#         - name: default
#           forwards:
#             - resource: service/my-service
#               protocol: tcp
#               port: 8080
#               localPort: 8080
#
`
	content := header + string(data)

	// Write with restrictive permissions (0600)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
