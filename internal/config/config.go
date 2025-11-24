package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	maxConfigSize = 10 * 1024 * 1024 // 10MB
)

// Config represents the root configuration structure from .kportal.yaml
type Config struct {
	Contexts []Context `yaml:"contexts"`
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

// Forward represents a single port-forward configuration
type Forward struct {
	Resource  string `yaml:"resource"`        // e.g., "pod/my-app", "service/postgres", "pod"
	Selector  string `yaml:"selector"`        // Label selector for pod resolution (e.g., "app=nginx,env=prod")
	Protocol  string `yaml:"protocol"`        // tcp or udp
	Port      int    `yaml:"port"`            // Remote port
	LocalPort int    `yaml:"localPort"`       // Local port
	Alias     string `yaml:"alias,omitempty"` // Optional human-readable alias for this forward

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

// LoadConfig loads and parses the configuration file from the given path.
func LoadConfig(path string) (*Config, error) {
	// Validate file size before reading
	fileInfo, err := os.Stat(path)
	if err != nil {
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
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
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
