package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidator_ValidateConfig(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		config        *Config
		name          string
		errorContains []string
		expectErrors  bool
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
			errorContains: []string{"Invalid protocol 'http'", "only 'tcp' is supported"},
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
		errorContains []string
		forward       Forward
		expectErrors  bool
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
		config        *Config
		name          string
		errorContains []string
		expectErrors  bool
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
		expectContains []string
		expectEmpty    bool
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
		config        *Config
		name          string
		errorContains []string
		expectErrors  bool
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
		config        *Config
		name          string
		errorContains []string
		expectErrors  bool
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
		{name: "valid simple", hostname: "myservice", valid: true},
		{name: "valid with hyphen", hostname: "my-service", valid: true},
		{name: "valid with numbers", hostname: "service123", valid: true},
		{name: "valid mixed", hostname: "my-service-123", valid: true},
		{name: "valid uppercase", hostname: "MyService", valid: true},
		{name: "valid single char", hostname: "a", valid: true},
		{name: "valid single digit", hostname: "1", valid: true},
		{name: "valid max length (63)", hostname: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", valid: true},
		{name: "invalid empty", hostname: "", valid: false},
		{name: "invalid too long (64)", hostname: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", valid: false},
		{name: "invalid starts with hyphen", hostname: "-myservice", valid: false},
		{name: "invalid ends with hyphen", hostname: "myservice-", valid: false},
		{name: "invalid underscore", hostname: "my_service", valid: false},
		{name: "invalid dot", hostname: "my.service", valid: false},
		{name: "invalid space", hostname: "my service", valid: false},
		{name: "invalid special char", hostname: "my@service", valid: false},
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
		{char: 'a', valid: true},
		{char: 'z', valid: true},
		{char: 'A', valid: true},
		{char: 'Z', valid: true},
		{char: '0', valid: true},
		{char: '9', valid: true},
		{char: '-', valid: false},
		{char: '_', valid: false},
		{char: '.', valid: false},
		{char: ' ', valid: false},
		{char: '@', valid: false},
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
		config       *Config
		name         string
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

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name        string
		portName    string
		port        int
		expectError bool
	}{
		{name: "valid port - minimum", portName: "port", port: 1, expectError: false},
		{name: "valid port - maximum", portName: "port", port: 65535, expectError: false},
		{name: "valid port - middle", portName: "port", port: 8080, expectError: false},
		{name: "invalid port - zero", portName: "port", port: 0, expectError: true},
		{name: "invalid port - negative", portName: "port", port: -1, expectError: true},
		{name: "invalid port - too high", portName: "port", port: 65536, expectError: true},
		{name: "invalid port - very high", portName: "localPort", port: 100000, expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port, tt.portName)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.portName)
				assert.Contains(t, err.Error(), fmt.Sprintf("%d", tt.port))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateResourceFormat(t *testing.T) {
	tests := []struct {
		name        string
		resource    string
		errorMsg    string
		expectError bool
	}{
		{name: "valid pod", resource: "pod/my-app", errorMsg: "", expectError: false},
		{name: "valid service", resource: "service/my-service", errorMsg: "", expectError: false},
		{name: "valid pod with subdomain", resource: "pod/my-app.example.com", errorMsg: "", expectError: false},
		{name: "missing slash", resource: "pod", errorMsg: "must be in format 'type/name'", expectError: true},
		{name: "empty string", resource: "", errorMsg: "must be in format 'type/name'", expectError: true},
		{name: "invalid type", resource: "deployment/my-app", errorMsg: "invalid resource type", expectError: true},
		{name: "empty name", resource: "pod/", errorMsg: "resource name cannot be empty", expectError: true},
		{name: "multiple slashes", resource: "pod/name/extra", errorMsg: "", expectError: false}, // First slash separates type/name, rest is part of name
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateResourceFormat(tt.resource)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateDuration(t *testing.T) {
	tests := []struct {
		name         string
		duration     string
		durationName string
		expectError  bool
	}{
		{name: "valid seconds", duration: "10s", durationName: "interval", expectError: false},
		{name: "valid minutes", duration: "5m", durationName: "timeout", expectError: false},
		{name: "valid hours", duration: "1h", durationName: "maxAge", expectError: false},
		{name: "valid milliseconds", duration: "500ms", durationName: "timeout", expectError: false},
		{name: "valid complex", duration: "1h30m", durationName: "duration", expectError: false},
		{name: "empty string", duration: "", durationName: "interval", expectError: false}, // Empty is allowed (uses default)
		{name: "invalid - no unit", duration: "10", durationName: "interval", expectError: true},
		{name: "invalid - bad format", duration: "abc", durationName: "timeout", expectError: true},
		{name: "invalid - unknown unit", duration: "10x", durationName: "interval", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDuration(tt.duration, tt.durationName)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.durationName)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateDNS1123Label(t *testing.T) {
	tests := []struct {
		name        string
		label       string
		errorMsg    string
		expectError bool
	}{
		{name: "valid simple", label: "myname", errorMsg: "", expectError: false},
		{name: "valid with hyphen", label: "my-name", errorMsg: "", expectError: false},
		{name: "valid with numbers", label: "name123", errorMsg: "", expectError: false},
		{name: "valid single char", label: "a", errorMsg: "", expectError: false},
		{name: "valid max length", label: strings.Repeat("a", 63), errorMsg: "", expectError: false},
		{name: "invalid empty", label: "", errorMsg: "not a valid DNS label", expectError: true},
		{name: "invalid uppercase", label: "MyName", errorMsg: "not a valid DNS label", expectError: true},
		{name: "invalid underscore", label: "my_name", errorMsg: "not a valid DNS label", expectError: true},
		{name: "invalid dot", label: "my.name", errorMsg: "not a valid DNS label", expectError: true},
		{name: "invalid starts with hyphen", label: "-name", errorMsg: "not a valid DNS label", expectError: true},
		{name: "invalid ends with hyphen", label: "name-", errorMsg: "not a valid DNS label", expectError: true},
		{name: "invalid too long", label: strings.Repeat("a", 64), errorMsg: "exceeds maximum length", expectError: true},
		{name: "invalid space", label: "my name", errorMsg: "not a valid DNS label", expectError: true},
		{name: "invalid special char", label: "name@", errorMsg: "not a valid DNS label", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDNS1123Label(tt.label, "test.field", "Test")
			if tt.expectError {
				assert.NotNil(t, err)
				assert.Contains(t, err.Message, tt.errorMsg)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestValidateDNS1123Subdomain(t *testing.T) {
	tests := []struct {
		name        string
		subdomain   string
		errorMsg    string
		expectError bool
	}{
		{name: "valid simple", subdomain: "myname", errorMsg: "", expectError: false},
		{name: "valid with hyphen", subdomain: "my-name", errorMsg: "", expectError: false},
		{name: "valid with dot", subdomain: "my.name", errorMsg: "", expectError: false},
		{name: "valid subdomain", subdomain: "app.example.com", errorMsg: "", expectError: false},
		{name: "valid with numbers", subdomain: "app123.example456", errorMsg: "", expectError: false},
		{name: "valid single char", subdomain: "a", errorMsg: "", expectError: false},
		{name: "valid max length", subdomain: strings.Repeat("a", 253), errorMsg: "", expectError: false},
		{name: "invalid empty", subdomain: "", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid uppercase", subdomain: "My.Name", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid underscore", subdomain: "my_name", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid starts with dot", subdomain: ".name", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid ends with dot", subdomain: "name.", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid double dot", subdomain: "my..name", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid starts with hyphen", subdomain: "-name", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid ends with hyphen", subdomain: "name-", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid too long", subdomain: strings.Repeat("a", 254), errorMsg: "exceeds maximum length", expectError: true},
		{name: "invalid space", subdomain: "my name", errorMsg: "not a valid DNS subdomain", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDNS1123Subdomain(tt.subdomain, "test.field", "Test")
			if tt.expectError {
				assert.NotNil(t, err)
				assert.Contains(t, err.Message, tt.errorMsg)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestValidator_ValidateSpecDurations(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		config        *Config
		name          string
		errorContains []string
		expectErrors  bool
	}{
		{
			name: "valid durations",
			config: &Config{
				HealthCheck: &HealthCheckSpec{
					Interval:         "5s",
					Timeout:          "2s",
					MaxConnectionAge: "25m",
					MaxIdleTime:      "10m",
					Method:           "tcp-dial",
				},
				Reliability: &ReliabilitySpec{
					TCPKeepalive:   "30s",
					DialTimeout:    "30s",
					WatchdogPeriod: "30s",
				},
			},
			expectErrors: false,
		},
		{
			name: "invalid health check interval",
			config: &Config{
				HealthCheck: &HealthCheckSpec{
					Interval: "invalid",
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid health check interval"},
		},
		{
			name: "invalid health check timeout",
			config: &Config{
				HealthCheck: &HealthCheckSpec{
					Timeout: "abc",
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid health check timeout"},
		},
		{
			name: "invalid max connection age",
			config: &Config{
				HealthCheck: &HealthCheckSpec{
					MaxConnectionAge: "xyz",
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid max connection age"},
		},
		{
			name: "invalid max idle time",
			config: &Config{
				HealthCheck: &HealthCheckSpec{
					MaxIdleTime: "bad",
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid max idle time"},
		},
		{
			name: "invalid health check method",
			config: &Config{
				HealthCheck: &HealthCheckSpec{
					Method: "invalid-method",
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid health check method"},
		},
		{
			name: "invalid TCP keepalive",
			config: &Config{
				Reliability: &ReliabilitySpec{
					TCPKeepalive: "not-a-duration",
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid TCP keepalive duration"},
		},
		{
			name: "invalid dial timeout",
			config: &Config{
				Reliability: &ReliabilitySpec{
					DialTimeout: "bad",
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid dial timeout"},
		},
		{
			name: "invalid watchdog period",
			config: &Config{
				Reliability: &ReliabilitySpec{
					WatchdogPeriod: "invalid",
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid watchdog period"},
		},
		{
			name: "multiple invalid durations",
			config: &Config{
				HealthCheck: &HealthCheckSpec{
					Interval: "bad",
					Timeout:  "worse",
				},
			},
			expectErrors:  true,
			errorContains: []string{"Invalid health check interval", "Invalid health check timeout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateSpecDurations(tt.config)

			if tt.expectErrors {
				assert.NotEmpty(t, errs, "expected validation errors")

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

func TestValidator_ValidateContextAndNamespaceNames(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		config        *Config
		name          string
		errorContains []string
		expectErrors  bool
	}{
		{
			name: "valid context and namespace names",
			config: &Config{
				Contexts: []Context{
					{
						Name: "my-cluster",
						Namespaces: []Namespace{
							{
								Name:     "default",
								Forwards: []Forward{{Resource: "pod/app", Port: 8080, LocalPort: 8080}},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "valid context with underscores (kubeconfig style)",
			config: &Config{
				Contexts: []Context{
					{
						Name: "gke_project_zone_cluster",
						Namespaces: []Namespace{
							{
								Name:     "my-namespace",
								Forwards: []Forward{{Resource: "pod/app", Port: 8080, LocalPort: 8080}},
							},
						},
					},
				},
			},
			expectErrors: false, // Context names now allow underscores
		},
		{
			name: "valid context with uppercase",
			config: &Config{
				Contexts: []Context{
					{
						Name: "MyCluster",
						Namespaces: []Namespace{
							{
								Name:     "default",
								Forwards: []Forward{{Resource: "pod/app", Port: 8080, LocalPort: 8080}},
							},
						},
					},
				},
			},
			expectErrors: false, // Context names now allow uppercase
		},
		{
			name: "valid namespace with dots (subdomain style)",
			config: &Config{
				Contexts: []Context{
					{
						Name: "my-cluster",
						Namespaces: []Namespace{
							{
								Name:     "my.app.example",
								Forwards: []Forward{{Resource: "pod/app", Port: 8080, LocalPort: 8080}},
							},
						},
					},
				},
			},
			expectErrors: false, // Namespaces now allow dots (DNS subdomain format)
		},
		{
			name: "invalid namespace name with uppercase",
			config: &Config{
				Contexts: []Context{
					{
						Name: "my-cluster",
						Namespaces: []Namespace{
							{
								Name:     "MyNamespace",
								Forwards: []Forward{{Resource: "pod/app", Port: 8080, LocalPort: 8080}},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"not a valid DNS subdomain"},
		},
		{
			name: "invalid context name with spaces",
			config: &Config{
				Contexts: []Context{
					{
						Name: "my cluster",
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
			errorContains: []string{"not valid", "alphanumeric"},
		},
		{
			name: "context name too long",
			config: &Config{
				Contexts: []Context{
					{
						Name: strings.Repeat("a", 254),
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
			errorContains: []string{"exceeds maximum length"},
		},
		{
			name: "invalid context name starts with hyphen",
			config: &Config{
				Contexts: []Context{
					{
						Name: "-mycluster",
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
			errorContains: []string{"not valid", "start/end with alphanumeric"},
		},
		{
			name: "invalid context name ends with underscore",
			config: &Config{
				Contexts: []Context{
					{
						Name: "mycluster_",
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
			errorContains: []string{"not valid", "start/end with alphanumeric"},
		},
		{
			name: "invalid namespace name with spaces",
			config: &Config{
				Contexts: []Context{
					{
						Name: "my-cluster",
						Namespaces: []Namespace{
							{
								Name:     "my namespace",
								Forwards: []Forward{{Resource: "pod/app", Port: 8080, LocalPort: 8080}},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"not a valid DNS subdomain"},
		},
		{
			name: "invalid namespace name with underscore",
			config: &Config{
				Contexts: []Context{
					{
						Name: "my-cluster",
						Namespaces: []Namespace{
							{
								Name:     "my_namespace",
								Forwards: []Forward{{Resource: "pod/app", Port: 8080, LocalPort: 8080}},
							},
						},
					},
				},
			},
			expectErrors:  true,
			errorContains: []string{"not a valid DNS subdomain"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.ValidateConfig(tt.config)

			if tt.expectErrors {
				assert.NotEmpty(t, errs, "expected validation errors")

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

func TestValidator_ValidateResourceNames(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name          string
		errorContains []string
		forward       Forward
		expectErrors  bool
	}{
		{
			name: "valid resource name",
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
			name: "valid resource name with subdomain",
			forward: Forward{
				Resource:      "service/my-service.example.com",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors: false,
		},
		{
			name: "invalid resource name with uppercase",
			forward: Forward{
				Resource:      "pod/MyApp",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors:  true,
			errorContains: []string{"not a valid DNS subdomain"},
		},
		{
			name: "invalid resource name with underscore",
			forward: Forward{
				Resource:      "pod/my_app",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors:  true,
			errorContains: []string{"not a valid DNS subdomain"},
		},
		{
			name: "invalid resource name starts with hyphen",
			forward: Forward{
				Resource:      "pod/-myapp",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors:  true,
			errorContains: []string{"not a valid DNS subdomain"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateForward(&tt.forward)

			if tt.expectErrors {
				assert.NotEmpty(t, errs, "expected validation errors")

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

func TestIsValidResourceType(t *testing.T) {
	tests := []struct {
		resourceType string
		expected     bool
	}{
		{resourceType: "pod", expected: true},
		{resourceType: "service", expected: true},
		{resourceType: "deployment", expected: false},
		{resourceType: "configmap", expected: false},
		{resourceType: "", expected: false},
		{resourceType: "POD", expected: false}, // case sensitive
		{resourceType: "Pod", expected: false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			result := isValidResourceType(tt.resourceType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidHealthCheckMethod(t *testing.T) {
	tests := []struct {
		method   string
		expected bool
	}{
		{method: "tcp-dial", expected: true},
		{method: "data-transfer", expected: true},
		{method: "ping", expected: false},
		{method: "http", expected: false},
		{method: "", expected: false},
		{method: "TCP-DIAL", expected: false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result := isValidHealthCheckMethod(tt.method)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidator_ValidateHTTPLog(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name          string
		errorContains []string
		forward       Forward
		expectErrors  bool
	}{
		{
			name: "valid HTTP log config",
			forward: Forward{
				Resource:      "pod/app",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
				HTTPLog: &HTTPLogSpec{
					Enabled:     true,
					MaxBodySize: 1024,
					LogFile:     "/tmp/test.log",
				},
			},
			expectErrors: false,
		},
		{
			name: "HTTP log disabled - no validation needed",
			forward: Forward{
				Resource:      "pod/app",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
				HTTPLog: &HTTPLogSpec{
					Enabled:     false,
					MaxBodySize: -1, // Would be invalid if enabled
				},
			},
			expectErrors: false,
		},
		{
			name: "no HTTP log config",
			forward: Forward{
				Resource:      "pod/app",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			},
			expectErrors: false,
		},
		{
			name: "invalid negative maxBodySize",
			forward: Forward{
				Resource:      "pod/app",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
				HTTPLog: &HTTPLogSpec{
					Enabled:     true,
					MaxBodySize: -1,
				},
			},
			expectErrors:  true,
			errorContains: []string{"maxBodySize", "non-negative"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateForward(&tt.forward)

			if tt.expectErrors {
				assert.NotEmpty(t, errs, "expected validation errors")

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

func TestValidateContextName(t *testing.T) {
	tests := []struct {
		name        string
		contextName string
		errorMsg    string
		expectError bool
	}{
		// Valid cases
		{name: "valid simple", contextName: "mycluster", errorMsg: "", expectError: false},
		{name: "valid with hyphen", contextName: "my-cluster", errorMsg: "", expectError: false},
		{name: "valid with underscore", contextName: "my_cluster", errorMsg: "", expectError: false},
		{name: "valid with numbers", contextName: "cluster123", errorMsg: "", expectError: false},
		{name: "valid mixed", contextName: "my-cluster_123", errorMsg: "", expectError: false},
		{name: "valid uppercase", contextName: "MyCluster", errorMsg: "", expectError: false},
		{name: "valid mixed case", contextName: "myCluster-Test_123", errorMsg: "", expectError: false},
		{name: "valid GKE style", contextName: "gke_project_us-central1_cluster", errorMsg: "", expectError: false},
		{name: "valid minikube", contextName: "minikube", errorMsg: "", expectError: false},
		{name: "valid docker desktop", contextName: "docker-desktop", errorMsg: "", expectError: false},
		{name: "valid docker desktop alt", contextName: "docker_desktop", errorMsg: "", expectError: false},
		{name: "valid single char", contextName: "a", errorMsg: "", expectError: false},
		{name: "valid single digit", contextName: "1", errorMsg: "", expectError: false},
		{name: "valid starts with digit", contextName: "123-cluster", errorMsg: "", expectError: false},

		// Invalid cases
		{name: "invalid empty", contextName: "", errorMsg: "not valid", expectError: true},
		{name: "invalid starts with hyphen", contextName: "-cluster", errorMsg: "not valid", expectError: true},
		{name: "invalid ends with hyphen", contextName: "cluster-", errorMsg: "not valid", expectError: true},
		{name: "invalid starts with underscore", contextName: "_cluster", errorMsg: "not valid", expectError: true},
		{name: "invalid ends with underscore", contextName: "cluster_", errorMsg: "not valid", expectError: true},
		{name: "invalid with spaces", contextName: "my cluster", errorMsg: "not valid", expectError: true},
		{name: "invalid with dots", contextName: "my.cluster", errorMsg: "not valid", expectError: true},
		{name: "invalid with special chars", contextName: "cluster@123", errorMsg: "not valid", expectError: true},
		{name: "invalid with slash", contextName: "cluster/name", errorMsg: "not valid", expectError: true},
		{name: "invalid too long", contextName: strings.Repeat("a", 254), errorMsg: "exceeds maximum length", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContextName(tt.contextName, "test.field")
			if tt.expectError {
				assert.NotNil(t, err)
				assert.Contains(t, err.Message, tt.errorMsg)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestValidateNamespaceName(t *testing.T) {
	tests := []struct {
		name        string
		namespace   string
		errorMsg    string
		expectError bool
	}{
		// Valid cases
		{name: "valid simple", namespace: "default", errorMsg: "", expectError: false},
		{name: "valid with hyphen", namespace: "kube-system", errorMsg: "", expectError: false},
		{name: "valid with dots", namespace: "my.app.example", errorMsg: "", expectError: false},
		{name: "valid subdomain", namespace: "app.ns.cluster.local", errorMsg: "", expectError: false},
		{name: "valid with numbers", namespace: "ns123", errorMsg: "", expectError: false},
		{name: "valid mixed", namespace: "my-app-123.test", errorMsg: "", expectError: false},
		{name: "valid single char", namespace: "a", errorMsg: "", expectError: false},
		{name: "valid single digit", namespace: "1", errorMsg: "", expectError: false},
		{name: "valid starts with digit", namespace: "123-ns", errorMsg: "", expectError: false},

		// Invalid cases
		{name: "invalid empty", namespace: "", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid starts with hyphen", namespace: "-namespace", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid ends with hyphen", namespace: "namespace-", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid with underscore", namespace: "my_namespace", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid with spaces", namespace: "my namespace", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid starts with dot", namespace: ".namespace", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid ends with dot", namespace: "namespace.", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid double dot", namespace: "my..namespace", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid uppercase", namespace: "MyNamespace", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid with special chars", namespace: "ns@123", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid with slash", namespace: "ns/name", errorMsg: "not a valid DNS subdomain", expectError: true},
		{name: "invalid too long", namespace: strings.Repeat("a", 254), errorMsg: "exceeds maximum length", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNamespaceName(tt.namespace, "test.field")
			if tt.expectError {
				assert.NotNil(t, err)
				assert.Contains(t, err.Message, tt.errorMsg)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestIsValidPort(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		expected bool
	}{
		// Valid ports
		{name: "valid minimum", port: 1, expected: true},
		{name: "valid maximum", port: 65535, expected: true},
		{name: "valid common", port: 8080, expected: true},
		{name: "valid HTTP", port: 80, expected: true},
		{name: "valid HTTPS", port: 443, expected: true},
		{name: "valid high", port: 30000, expected: true},

		// Invalid ports
		{name: "invalid zero", port: 0, expected: false},
		{name: "invalid negative", port: -1, expected: false},
		{name: "invalid too high", port: 65536, expected: false},
		{name: "invalid very high", port: 100000, expected: false},
		{name: "invalid negative large", port: -8080, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidPort(tt.port)
			assert.Equal(t, tt.expected, result, "IsValidPort(%d) = %v, want %v", tt.port, result, tt.expected)
		})
	}
}

func TestValidateProtocol(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name          string
		protocol      string
		errorContains string
		expectErrors  bool
	}{
		// Valid protocols
		{name: "valid tcp", protocol: "tcp", errorContains: "", expectErrors: false},
		{name: "valid empty", protocol: "", errorContains: "", expectErrors: false},

		// Invalid protocols
		{name: "invalid udp", protocol: "udp", errorContains: "only 'tcp' is supported", expectErrors: true},
		{name: "invalid http", protocol: "http", errorContains: "only 'tcp' is supported", expectErrors: true},
		{name: "invalid https", protocol: "https", errorContains: "only 'tcp' is supported", expectErrors: true},
		{name: "invalid uppercase TCP", protocol: "TCP", errorContains: "only 'tcp' is supported", expectErrors: true},
		{name: "invalid mixed case", protocol: "Tcp", errorContains: "only 'tcp' is supported", expectErrors: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fwd := Forward{
				Resource:      "pod/my-app",
				Protocol:      tt.protocol,
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev",
				namespaceName: "default",
			}
			errs := validator.validateForward(&fwd)

			if tt.expectErrors {
				assert.NotEmpty(t, errs, "expected validation errors")
				found := false
				for _, err := range errs {
					if strings.Contains(err.Message, tt.errorContains) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error message '%s' not found in errors: %v", tt.errorContains, errs)
			} else {
				assert.Empty(t, errs, "expected no validation errors, got: %v", errs)
			}
		})
	}
}
