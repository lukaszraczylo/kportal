package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidator_ValidateConfig(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name          string
		config        *Config
		expectErrors  bool
		errorContains []string
	}{
		{
			name: "valid config",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "pod/my-app",
										Protocol:      "tcp",
										Port:          8080,
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name:          "nil config",
			config:        nil,
			expectErrors:  true,
			errorContains: []string{"Configuration is nil"},
		},
		{
			name: "empty contexts",
			config: &Config{
				Contexts: []Context{},
			},
			expectErrors:  true,
			errorContains: []string{"must have at least one context"},
		},
		{
			name: "empty namespaces",
			config: &Config{
				Contexts: []Context{
					{
						Name:       "dev-cluster",
						Namespaces: []Namespace{},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"must have at least one namespace"},
		},
		{
			name: "empty forwards",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name:     "default",
								Forwards: []Forward{},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"must have at least one forward"},
		},
		{
			name: "invalid port - zero",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "pod/my-app",
										Protocol:      "tcp",
										Port:          0,
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid port 0"},
		},
		{
			name: "invalid port - above max",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "pod/my-app",
										Protocol:      "tcp",
										Port:          8080,
										LocalPort:     65536,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid localPort 65536"},
		},
		{
			name: "invalid protocol",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "pod/my-app",
										Protocol:      "http",
										Port:          8080,
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid protocol 'http'", "must be 'tcp' or 'udp'"},
		},
		{
			name: "empty resource",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "",
										Protocol:      "tcp",
										Port:          8080,
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"Resource cannot be empty"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.ValidateConfig(tt.config)

			if tt.expectErrors {
				assert.NotEmpty(t, errs, "expected validation errors")

				// Check that expected error messages are present
				for _, expectedMsg := range tt.errorContains {
					found := false
					for _, err := range errs {
						if strings.Contains(err.Message, expectedMsg) {
							found = true
							break
						}
					}
					assert.True(t, found, "expected error message '%s' not found", expectedMsg)
				}
			} else {
				assert.Empty(t, errs, "expected no validation errors")
			}
		})
	}
}

func TestValidator_ValidateResourceFormat(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name          string
		forward       Forward
		expectErrors  bool
		errorContains []string
	}{
		{
			name: "valid pod with name",
			forward: Forward{
				Resource:      "pod/my-app",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors: false,
		},
		{
			name: "valid service with name",
			forward: Forward{
				Resource:      "service/postgres",
				Port:          5432,
				LocalPort:     5432,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors: false,
		},
		{
			name: "valid pod with selector (no name)",
			forward: Forward{
				Resource:      "pod",
				Selector:      "app=nginx",
				Port:          80,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors: false,
		},
		{
			name: "invalid resource type",
			forward: Forward{
				Resource:      "deployment/my-app",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors:  true,
			errorContains: []string{"Invalid resource type 'deployment'"},
		},
		{
			name: "pod with name and selector (invalid)",
			forward: Forward{
				Resource:      "pod/my-app",
				Selector:      "app=nginx",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors:  true,
			errorContains: []string{"should not have a selector"},
		},
		{
			name: "pod without name and without selector (invalid)",
			forward: Forward{
				Resource:      "pod",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors:  true,
			errorContains: []string{"must have a selector"},
		},
		{
			name: "service without name (invalid)",
			forward: Forward{
				Resource:      "service",
				Port:          5432,
				LocalPort:     5432,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors:  true,
			errorContains: []string{"Service name cannot be empty"},
		},
		{
			name: "service with selector (invalid)",
			forward: Forward{
				Resource:      "service/postgres",
				Selector:      "app=db",
				Port:          5432,
				LocalPort:     5432,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors:  true,
			errorContains: []string{"should not have a selector"},
		},
		{
			name: "pod with empty name after slash",
			forward: Forward{
				Resource:      "pod/",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors:  true,
			errorContains: []string{"Pod name cannot be empty"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateForward(&tt.forward)

			if tt.expectErrors {
				assert.NotEmpty(t, errs, "expected validation errors")

				// Check that expected error messages are present
				for _, expectedMsg := range tt.errorContains {
					found := false
					for _, err := range errs {
						if strings.Contains(err.Message, expectedMsg) {
							found = true
							break
						}
					}
					assert.True(t, found, "expected error message '%s' not found", expectedMsg)
				}
			} else {
				assert.Empty(t, errs, "expected no validation errors")
			}
		})
	}
}

func TestValidator_CheckDuplicatePorts(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name          string
		config        *Config
		expectErrors  bool
		errorContains []string
	}{
		{
			name: "no duplicate ports",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "pod/app1",
										Port:          8080,
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
									{
										Resource:      "pod/app2",
										Port:          8081,
										LocalPort:     8081,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "duplicate ports in same namespace",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "pod/app1",
										Port:          8080,
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
									{
										Resource:      "pod/app2",
										Port:          8081,
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"Duplicate local port 8080"},
		},
		{
			name: "duplicate ports across namespaces",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name: "ns1",
								Forwards: []Forward{
									{
										Resource:      "pod/app1",
										Port:          8080,
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "ns1",
									},
								},
							},
							{
								Name: "ns2",
								Forwards: []Forward{
									{
										Resource:      "pod/app2",
										Port:          8080,
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "ns2",
									},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"Duplicate local port 8080"},
		},
		{
			name: "duplicate ports across contexts",
			config: &Config{
				Contexts: []Context{
					{
						Name: "cluster1",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "pod/app1",
										Port:          8080,
										LocalPort:     8080,
										contextName:   "cluster1",
										namespaceName: "default",
									},
								},
							},
						},
					},
					{
						Name: "cluster2",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "pod/app2",
										Port:          8080,
										LocalPort:     8080,
										contextName:   "cluster2",
										namespaceName: "default",
									},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"Duplicate local port 8080"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateDuplicatePorts(tt.config)

			if tt.expectErrors {
				assert.NotEmpty(t, errs, "expected validation errors")

				// Check that expected error messages are present
				for _, expectedMsg := range tt.errorContains {
					found := false
					for _, err := range errs {
						if strings.Contains(err.Message, expectedMsg) {
							found = true
							break
						}
					}
					assert.True(t, found, "expected error message '%s' not found", expectedMsg)
				}
			} else {
				assert.Empty(t, errs, "expected no validation errors")
			}
		})
	}
}

func TestFormatValidationErrors(t *testing.T) {
	tests := []struct {
		name           string
		errors         []ValidationError
		expectEmpty    bool
		expectContains []string
	}{
		{
			name:        "no errors",
			errors:      []ValidationError{},
			expectEmpty: true,
		},
		{
			name: "single error",
			errors: []ValidationError{
				{
					Field:   "port",
					Message: "Invalid port 0",
				},
			},
			expectEmpty:    false,
			expectContains: []string{"Configuration Validation Errors", "1. Invalid port 0"},
		},
		{
			name: "multiple errors",
			errors: []ValidationError{
				{
					Field:   "port",
					Message: "Invalid port 0",
				},
				{
					Field:   "resource",
					Message: "Resource cannot be empty",
				},
			},
			expectEmpty:    false,
			expectContains: []string{"Configuration Validation Errors", "1. Invalid port 0", "2. Resource cannot be empty"},
		},
		{
			name: "error with context",
			errors: []ValidationError{
				{
					Field:   "localPort",
					Message: "Duplicate local port 8080",
					Context: map[string]string{
						"port":     "8080",
						"forwards": "dev/default/pod/app1:8080, dev/default/pod/app2:8080",
					},
				},
			},
			expectEmpty:    false,
			expectContains: []string{"Configuration Validation Errors", "Duplicate local port 8080", "port:", "8080", "forwards:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := FormatValidationErrors(tt.errors)

			if tt.expectEmpty {
				assert.Empty(t, output, "expected empty output")
			} else {
				assert.NotEmpty(t, output, "expected non-empty output")

				// Check that expected strings are present
				for _, expected := range tt.expectContains {
					assert.Contains(t, output, expected, "output should contain '%s'", expected)
				}
			}
		})
	}
}

func TestValidator_ValidateStructure(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name          string
		config        *Config
		expectErrors  bool
		errorContains []string
	}{
		{
			name: "empty context name",
			config: &Config{
				Contexts: []Context{
					{
						Name: "",
						Namespaces: []Namespace{
							{
								Name:     "default",
								Forwards: []Forward{{Resource: "pod/app", Port: 8080, LocalPort: 8080}},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"Context name cannot be empty"},
		},
		{
			name: "empty namespace name",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev",
						Namespaces: []Namespace{
							{
								Name:     "",
								Forwards: []Forward{{Resource: "pod/app", Port: 8080, LocalPort: 8080}},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"Namespace name cannot be empty"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateStructure(tt.config)

			if tt.expectErrors {
				assert.NotEmpty(t, errs, "expected validation errors")

				// Check that expected error messages are present
				for _, expectedMsg := range tt.errorContains {
					found := false
					for _, err := range errs {
						if strings.Contains(err.Message, expectedMsg) {
							found = true
							break
						}
					}
					assert.True(t, found, "expected error message '%s' not found", expectedMsg)
				}
			} else {
				assert.Empty(t, errs, "expected no validation errors")
			}
		})
	}
}

func TestValidator_ValidateMDNS(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name          string
		config        *Config
		expectErrors  bool
		errorContains []string
	}{
		{
			name: "mDNS disabled - no validation",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{Resource: "pod/app", Port: 8080, LocalPort: 8080, Alias: "invalid_alias", contextName: "dev", namespaceName: "default"},
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "mDNS enabled - valid aliases",
			config: &Config{
				MDNS: &MDNSSpec{Enabled: true},
				Contexts: []Context{
					{
						Name: "dev",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{Resource: "pod/app1", Port: 8080, LocalPort: 8080, Alias: "my-app", contextName: "dev", namespaceName: "default"},
									{Resource: "pod/app2", Port: 8081, LocalPort: 8081, Alias: "my-service", contextName: "dev", namespaceName: "default"},
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "mDNS enabled - no alias (allowed)",
			config: &Config{
				MDNS: &MDNSSpec{Enabled: true},
				Contexts: []Context{
					{
						Name: "dev",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{Resource: "pod/app", Port: 8080, LocalPort: 8080, contextName: "dev", namespaceName: "default"},
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "mDNS enabled - invalid alias with underscore",
			config: &Config{
				MDNS: &MDNSSpec{Enabled: true},
				Contexts: []Context{
					{
						Name: "dev",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{Resource: "pod/app", Port: 8080, LocalPort: 8080, Alias: "my_app", contextName: "dev", namespaceName: "default"},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"invalid mDNS hostname", "RFC 1123"},
		},
		{
			name: "mDNS enabled - alias starts with hyphen",
			config: &Config{
				MDNS: &MDNSSpec{Enabled: true},
				Contexts: []Context{
					{
						Name: "dev",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{Resource: "pod/app", Port: 8080, LocalPort: 8080, Alias: "-myapp", contextName: "dev", namespaceName: "default"},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"invalid mDNS hostname"},
		},
		{
			name: "mDNS enabled - alias ends with hyphen",
			config: &Config{
				MDNS: &MDNSSpec{Enabled: true},
				Contexts: []Context{
					{
						Name: "dev",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{Resource: "pod/app", Port: 8080, LocalPort: 8080, Alias: "myapp-", contextName: "dev", namespaceName: "default"},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"invalid mDNS hostname"},
		},
		{
			name: "mDNS enabled - duplicate aliases",
			config: &Config{
				MDNS: &MDNSSpec{Enabled: true},
				Contexts: []Context{
					{
						Name: "dev",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{Resource: "pod/app1", Port: 8080, LocalPort: 8080, Alias: "myapp", contextName: "dev", namespaceName: "default"},
									{Resource: "pod/app2", Port: 8081, LocalPort: 8081, Alias: "myapp", contextName: "dev", namespaceName: "default"},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"Duplicate mDNS hostname", "conflict"},
		},
		{
			name: "mDNS enabled - duplicate aliases across contexts",
			config: &Config{
				MDNS: &MDNSSpec{Enabled: true},
				Contexts: []Context{
					{
						Name: "cluster1",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{Resource: "pod/app1", Port: 8080, LocalPort: 8080, Alias: "shared-name", contextName: "cluster1", namespaceName: "default"},
								},
							},
						},
					},
					{
						Name: "cluster2",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{Resource: "pod/app2", Port: 8081, LocalPort: 8081, Alias: "shared-name", contextName: "cluster2", namespaceName: "default"},
								},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"Duplicate mDNS hostname", "shared-name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.ValidateConfig(tt.config)

			if tt.expectErrors {
				assert.NotEmpty(t, errs, "expected validation errors")

				// Check that expected error messages are present
				for _, expectedMsg := range tt.errorContains {
					found := false
					for _, err := range errs {
						if strings.Contains(err.Message, expectedMsg) {
							found = true
							break
						}
					}
					assert.True(t, found, "expected error message '%s' not found in errors: %v", expectedMsg, errs)
				}
			} else {
				assert.Empty(t, errs, "expected no validation errors, got: %v", errs)
			}
		})
	}
}

func TestIsValidHostname(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		valid    bool
	}{
		{"valid simple", "myservice", true},
		{"valid with hyphen", "my-service", true},
		{"valid with numbers", "service123", true},
		{"valid mixed", "my-service-123", true},
		{"valid uppercase", "MyService", true},
		{"valid single char", "a", true},
		{"valid single digit", "1", true},
		{"valid max length (63)", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"invalid empty", "", false},
		{"invalid too long (64)", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"invalid starts with hyphen", "-myservice", false},
		{"invalid ends with hyphen", "myservice-", false},
		{"invalid underscore", "my_service", false},
		{"invalid dot", "my.service", false},
		{"invalid space", "my service", false},
		{"invalid special char", "my@service", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidHostname(tt.hostname)
			assert.Equal(t, tt.valid, result, "isValidHostname(%q) = %v, want %v", tt.hostname, result, tt.valid)
		})
	}
}

func TestIsAlphanumeric(t *testing.T) {
	tests := []struct {
		char  byte
		valid bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'-', false},
		{'_', false},
		{'.', false},
		{' ', false},
		{'@', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			result := isAlphanumeric(tt.char)
			assert.Equal(t, tt.valid, result, "isAlphanumeric(%q) = %v, want %v", tt.char, result, tt.valid)
		})
	}
}

func TestValidator_ValidateConfigWithOptions(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name         string
		config       *Config
		allowEmpty   bool
		expectErrors bool
	}{
		{
			name:         "empty config - strict mode",
			config:       &Config{Contexts: []Context{}},
			allowEmpty:   false,
			expectErrors: true,
		},
		{
			name:         "empty config - allow empty",
			config:       &Config{Contexts: []Context{}},
			allowEmpty:   true,
			expectErrors: false,
		},
		{
			name:         "nil contexts - allow empty",
			config:       &Config{},
			allowEmpty:   true,
			expectErrors: false,
		},
		{
			name: "context with no forwards - allow empty",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev",
						Namespaces: []Namespace{
							{Name: "default", Forwards: []Forward{}},
						},
					},
				},
			},
			allowEmpty:   true,
			expectErrors: false,
		},
		{
			name: "valid config - strict mode",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "pod/my-app",
										Protocol:      "tcp",
										Port:          8080,
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
								},
							},
						},
					},
				},
			},
			allowEmpty:   false,
			expectErrors: false,
		},
		{
			name: "valid config - allow empty (should still validate)",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "pod/my-app",
										Protocol:      "tcp",
										Port:          8080,
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
								},
							},
						},
					},
				},
			},
			allowEmpty:   true,
			expectErrors: false,
		},
		{
			name: "invalid forward in non-empty config - allow empty still validates",
			config: &Config{
				Contexts: []Context{
					{
						Name: "dev-cluster",
						Namespaces: []Namespace{
							{
								Name: "default",
								Forwards: []Forward{
									{
										Resource:      "pod/my-app",
										Protocol:      "tcp",
										Port:          0, // Invalid port
										LocalPort:     8080,
										contextName:   "dev-cluster",
										namespaceName: "default",
									},
								},
							},
						},
					},
				},
			},
			allowEmpty:   true,
			expectErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.ValidateConfigWithOptions(tt.config, tt.allowEmpty)

			if tt.expectErrors {
				assert.NotEmpty(t, errs, "expected validation errors")
			} else {
				assert.Empty(t, errs, "expected no validation errors, got: %v", errs)
			}
		})
	}
}
