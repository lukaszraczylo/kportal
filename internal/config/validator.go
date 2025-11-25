package config

import (
	"fmt"
	"strings"
)

const (
	MinPort = 1
	MaxPort = 65535
)

// IsValidPort returns true if the port number is within the valid range (1-65535).
func IsValidPort(port int) bool {
	return port >= MinPort && port <= MaxPort
}

// ValidationError represents a configuration validation error with context.
type ValidationError struct {
	Field   string            // The field that failed validation
	Message string            // Error message
	Context map[string]string // Additional context information
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	return e.Message
}

// Validator validates configuration files.
type Validator struct{}

// NewValidator creates a new Validator instance.
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateConfig validates the entire configuration and returns all errors found.
func (v *Validator) ValidateConfig(cfg *Config) []ValidationError {
	var errs []ValidationError

	if cfg == nil {
		return []ValidationError{{
			Field:   "config",
			Message: "Configuration is nil",
		}}
	}

	// Validate structure
	errs = append(errs, v.validateStructure(cfg)...)

	// Validate each forward
	for _, ctx := range cfg.Contexts {
		for _, ns := range ctx.Namespaces {
			for _, fwd := range ns.Forwards {
				errs = append(errs, v.validateForward(&fwd)...)
			}
		}
	}

	// Check for duplicate local ports
	errs = append(errs, v.validateDuplicatePorts(cfg)...)

	// Validate mDNS configuration
	if cfg.IsMDNSEnabled() {
		errs = append(errs, v.validateMDNS(cfg)...)
	}

	return errs
}

// validateStructure validates the basic structure of the configuration.
func (v *Validator) validateStructure(cfg *Config) []ValidationError {
	var errs []ValidationError

	if len(cfg.Contexts) == 0 {
		errs = append(errs, ValidationError{
			Field:   "contexts",
			Message: "Configuration must have at least one context",
		})
		return errs
	}

	for i, ctx := range cfg.Contexts {
		if ctx.Name == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("contexts[%d].name", i),
				Message: "Context name cannot be empty",
			})
		}

		if len(ctx.Namespaces) == 0 {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("contexts[%d].namespaces", i),
				Message: fmt.Sprintf("Context '%s' must have at least one namespace", ctx.Name),
			})
			// Don't continue - still validate other aspects of the context if any
		}

		for j, ns := range ctx.Namespaces {
			if ns.Name == "" {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("contexts[%d].namespaces[%d].name", i, j),
					Message: fmt.Sprintf("Namespace name cannot be empty in context '%s'", ctx.Name),
				})
			}

			if len(ns.Forwards) == 0 {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("contexts[%d].namespaces[%d].forwards", i, j),
					Message: fmt.Sprintf("Namespace '%s/%s' must have at least one forward", ctx.Name, ns.Name),
				})
			}
		}
	}

	return errs
}

// validateForward validates a single forward configuration.
func (v *Validator) validateForward(fwd *Forward) []ValidationError {
	var errs []ValidationError

	// Validate resource
	if fwd.Resource == "" {
		errs = append(errs, ValidationError{
			Field:   "resource",
			Message: fmt.Sprintf("Resource cannot be empty for forward %s", fwd.ID()),
		})
	} else {
		errs = append(errs, v.validateResource(fwd)...)
	}

	// Validate protocol
	if fwd.Protocol != "" && fwd.Protocol != "tcp" && fwd.Protocol != "udp" {
		errs = append(errs, ValidationError{
			Field:   "protocol",
			Message: fmt.Sprintf("Invalid protocol '%s' for forward %s (must be 'tcp' or 'udp')", fwd.Protocol, fwd.ID()),
		})
	}

	// Validate ports
	if fwd.Port < MinPort || fwd.Port > MaxPort {
		errs = append(errs, ValidationError{
			Field:   "port",
			Message: fmt.Sprintf("Invalid port %d for forward %s (must be between %d and %d)", fwd.Port, fwd.ID(), MinPort, MaxPort),
		})
	}

	if fwd.LocalPort < MinPort || fwd.LocalPort > MaxPort {
		errs = append(errs, ValidationError{
			Field:   "localPort",
			Message: fmt.Sprintf("Invalid localPort %d for forward %s (must be between %d and %d)", fwd.LocalPort, fwd.ID(), MinPort, MaxPort),
		})
	}

	return errs
}

// validateResource validates the resource field format and selector usage.
func (v *Validator) validateResource(fwd *Forward) []ValidationError {
	var errs []ValidationError

	parts := strings.SplitN(fwd.Resource, "/", 2)
	resourceType := parts[0]

	// Valid resource types: pod, service
	if resourceType != "pod" && resourceType != "service" {
		errs = append(errs, ValidationError{
			Field:   "resource",
			Message: fmt.Sprintf("Invalid resource type '%s' for forward %s (must be 'pod' or 'service')", resourceType, fwd.ID()),
		})
		return errs
	}

	// For pod resources
	if resourceType == "pod" {
		if len(parts) == 2 {
			// pod/name format - should not have selector
			if fwd.Selector != "" {
				errs = append(errs, ValidationError{
					Field:   "selector",
					Message: fmt.Sprintf("Forward %s uses explicit pod name (%s) and should not have a selector", fwd.ID(), fwd.Resource),
				})
			}

			// Validate pod name is not empty
			if parts[1] == "" {
				errs = append(errs, ValidationError{
					Field:   "resource",
					Message: fmt.Sprintf("Pod name cannot be empty for forward %s", fwd.ID()),
				})
			}
		} else {
			// pod (no name) - must have selector
			if fwd.Selector == "" {
				errs = append(errs, ValidationError{
					Field:   "selector",
					Message: fmt.Sprintf("Forward %s uses generic 'pod' resource and must have a selector", fwd.ID()),
				})
			}
		}
	}

	// For service resources
	if resourceType == "service" {
		if len(parts) < 2 || parts[1] == "" {
			errs = append(errs, ValidationError{
				Field:   "resource",
				Message: fmt.Sprintf("Service name cannot be empty for forward %s", fwd.ID()),
			})
		}

		if fwd.Selector != "" {
			errs = append(errs, ValidationError{
				Field:   "selector",
				Message: fmt.Sprintf("Forward %s uses service resource and should not have a selector", fwd.ID()),
			})
		}
	}

	return errs
}

// validateDuplicatePorts checks for duplicate local ports across all forwards.
func (v *Validator) validateDuplicatePorts(cfg *Config) []ValidationError {
	var errs []ValidationError

	portMap := make(map[int][]string) // port -> list of forward IDs

	for _, ctx := range cfg.Contexts {
		for _, ns := range ctx.Namespaces {
			for _, fwd := range ns.Forwards {
				portMap[fwd.LocalPort] = append(portMap[fwd.LocalPort], fwd.ID())
			}
		}
	}

	// Find duplicates
	for port, forwards := range portMap {
		if len(forwards) > 1 {
			errs = append(errs, ValidationError{
				Field:   "localPort",
				Message: fmt.Sprintf("Duplicate local port %d used by multiple forwards", port),
				Context: map[string]string{
					"port":     fmt.Sprintf("%d", port),
					"forwards": strings.Join(forwards, ", "),
				},
			})
		}
	}

	return errs
}

// FormatValidationErrors formats validation errors into a human-readable string.
func FormatValidationErrors(errs []ValidationError) string {
	if len(errs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\nConfiguration Validation Errors:\n")
	sb.WriteString(strings.Repeat("=", 50) + "\n\n")

	for i, err := range errs {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, err.Message))
		if len(err.Context) > 0 {
			for k, v := range err.Context {
				sb.WriteString(fmt.Sprintf("   %s: %s\n", k, v))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// validateMDNS validates mDNS configuration when enabled.
// It checks that aliases used for mDNS hostnames are valid and unique.
// This includes both explicit aliases and auto-generated ones from resource names.
func (v *Validator) validateMDNS(cfg *Config) []ValidationError {
	var errs []ValidationError

	aliasMap := make(map[string][]string) // alias -> list of forward IDs using it

	for _, ctx := range cfg.Contexts {
		for _, ns := range ctx.Namespaces {
			for _, fwd := range ns.Forwards {
				// Get the mDNS alias (explicit or generated from resource name)
				mdnsAlias := fwd.GetMDNSAlias()
				if mdnsAlias == "" {
					// No alias available (e.g., "pod" with selector only)
					continue
				}

				// Validate alias is a valid hostname (RFC 1123)
				if !isValidHostname(mdnsAlias) {
					errs = append(errs, ValidationError{
						Field:   "alias",
						Message: fmt.Sprintf("Forward %s has invalid mDNS hostname '%s' (must be a valid RFC 1123 hostname)", fwd.ID(), mdnsAlias),
					})
				}

				aliasMap[mdnsAlias] = append(aliasMap[mdnsAlias], fwd.ID())
			}
		}
	}

	// Check for duplicate aliases (would cause mDNS conflicts)
	for alias, forwards := range aliasMap {
		if len(forwards) > 1 {
			errs = append(errs, ValidationError{
				Field:   "alias",
				Message: fmt.Sprintf("Duplicate mDNS hostname '%s' used by multiple forwards (would cause conflict)", alias),
				Context: map[string]string{
					"alias":    alias,
					"forwards": strings.Join(forwards, ", "),
				},
			})
		}
	}

	return errs
}

// isValidHostname checks if a string is a valid RFC 1123 hostname.
// Hostnames must start with alphanumeric, contain only alphanumeric and hyphens,
// and be 1-63 characters long.
func isValidHostname(name string) bool {
	if len(name) == 0 || len(name) > 63 {
		return false
	}

	// Must start with alphanumeric
	if !isAlphanumeric(name[0]) {
		return false
	}

	// Must end with alphanumeric
	if !isAlphanumeric(name[len(name)-1]) {
		return false
	}

	// Check all characters
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !isAlphanumeric(c) && c != '-' {
			return false
		}
	}

	return true
}

// isAlphanumeric returns true if the character is a letter or digit.
func isAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
