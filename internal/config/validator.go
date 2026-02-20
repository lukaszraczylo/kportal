package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	MinPort = 1
	MaxPort = 65535

	// DNS1123LabelMaxLength is the maximum length of a DNS label (RFC 1123)
	DNS1123LabelMaxLength = 63
	// DNS1123SubdomainMaxLength is the maximum length of a DNS subdomain name
	DNS1123SubdomainMaxLength = 253
)

var (
	// dns1123LabelRegexp matches valid DNS labels (RFC 1123)
	// Must consist of lowercase alphanumeric characters or '-', start with alphanumeric, end with alphanumeric
	dns1123LabelRegexp = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

	// dns1123SubdomainRegexp matches valid DNS subdomain names
	// A series of DNS labels separated by dots (no consecutive dots allowed)
	dns1123SubdomainRegexp = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

	// contextNameRegexp matches valid context names
	// Allows alphanumeric characters, hyphens, and underscores (to support various kubeconfig naming conventions)
	contextNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_-]*[a-zA-Z0-9])?$`)

	// validResourceTypes contains the allowed Kubernetes resource types
	validResourceTypes = []string{"pod", "service"}

	// validHealthCheckMethods contains the allowed health check methods
	validHealthCheckMethods = []string{"tcp-dial", "data-transfer"}
)

// IsValidPort returns true if the port number is within the valid range (1-65535).
func IsValidPort(port int) bool {
	return port >= MinPort && port <= MaxPort
}

// ValidationError represents a configuration validation error with context.
type ValidationError struct {
	Context map[string]string
	Field   string
	Message string
}

// Validator validates configuration files.
type Validator struct{}

// NewValidator creates a new Validator instance.
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateConfig validates the entire configuration and returns all errors found.
func (v *Validator) ValidateConfig(cfg *Config) []ValidationError {
	return v.ValidateConfigWithOptions(cfg, false)
}

// ValidateConfigWithOptions validates configuration with configurable strictness.
// When allowEmpty is true, empty configurations (no contexts/forwards) are allowed.
// This is useful for newly created config files where the user will add forwards via the TUI.
func (v *Validator) ValidateConfigWithOptions(cfg *Config, allowEmpty bool) []ValidationError {
	var errs []ValidationError

	if cfg == nil {
		return []ValidationError{{
			Field:   "config",
			Message: "Configuration is nil",
		}}
	}

	// If empty configs are allowed and this config is empty, skip structure validation
	if allowEmpty && cfg.IsEmpty() {
		// Still validate health check and reliability if present (they don't require forwards)
		errs = append(errs, v.validateSpecDurations(cfg)...)
		return errs
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

	// Validate duration fields in specs
	errs = append(errs, v.validateSpecDurations(cfg)...)

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
		} else {
			// Validate context name format (alphanumeric, hyphens, underscores)
			if err := validateContextName(ctx.Name, fmt.Sprintf("contexts[%d].name", i)); err != nil {
				errs = append(errs, *err)
			}
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
			} else {
				// Validate namespace name follows DNS subdomain conventions (Kubernetes requirement)
				if err := validateNamespaceName(ns.Name, fmt.Sprintf("contexts[%d].namespaces[%d].name", i, j)); err != nil {
					errs = append(errs, *err)
				}
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

	// Validate protocol - only "tcp" is currently supported
	if fwd.Protocol != "" && fwd.Protocol != "tcp" {
		errs = append(errs, ValidationError{
			Field:   "protocol",
			Message: fmt.Sprintf("Invalid protocol '%s' for forward %s (only 'tcp' is supported)", fwd.Protocol, fwd.ID()),
		})
	}

	// Validate ports
	if !IsValidPort(fwd.Port) {
		errs = append(errs, ValidationError{
			Field:   "port",
			Message: fmt.Sprintf("Invalid port %d for forward %s (must be between %d and %d)", fwd.Port, fwd.ID(), MinPort, MaxPort),
		})
	}

	if !IsValidPort(fwd.LocalPort) {
		errs = append(errs, ValidationError{
			Field:   "localPort",
			Message: fmt.Sprintf("Invalid localPort %d for forward %s (must be between %d and %d)", fwd.LocalPort, fwd.ID(), MinPort, MaxPort),
		})
	}

	// Note: Alias validation is handled in validateMDNS since aliases are primarily
	// used for mDNS hostname registration. We only validate alias format when mDNS
	// is enabled to avoid unnecessary restrictions on non-mDNS usage.

	// Validate HTTP log configuration if enabled
	if fwd.HTTPLog != nil && fwd.HTTPLog.Enabled {
		errs = append(errs, v.validateHTTPLog(fwd)...)
	}

	return errs
}

// validateResource validates the resource field format and selector usage.
func (v *Validator) validateResource(fwd *Forward) []ValidationError {
	var errs []ValidationError

	// Validate resource format (must be "type/name" or just "type" for pod with selector)
	parts := strings.SplitN(fwd.Resource, "/", 2)
	resourceType := parts[0]

	// Validate resource type
	if !isValidResourceType(resourceType) {
		errs = append(errs, ValidationError{
			Field:   "resource",
			Message: fmt.Sprintf("Invalid resource type '%s' for forward %s (must be one of: %s)", resourceType, fwd.ID(), strings.Join(validResourceTypes, ", ")),
		})
		return errs
	}

	// Validate resource name if provided
	if len(parts) == 2 {
		resourceName := parts[1]
		if resourceName == "" {
			// Use resource-type-specific error message for better clarity
			entityType := "Resource"
			switch resourceType {
			case "pod":
				entityType = "Pod"
			case "service":
				entityType = "Service"
			}
			errs = append(errs, ValidationError{
				Field:   "resource",
				Message: fmt.Sprintf("%s name cannot be empty for forward %s", entityType, fwd.ID()),
			})
		} else {
			// Validate resource name follows DNS subdomain conventions
			if err := validateDNS1123Subdomain(resourceName, "resource", "Resource name"); err != nil {
				err.Message = fmt.Sprintf("%s for forward %s", err.Message, fwd.ID())
				errs = append(errs, *err)
			}
		}
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
		} else if fwd.Selector == "" {
			// pod (no name) - must have selector
			errs = append(errs, ValidationError{
				Field:   "selector",
				Message: fmt.Sprintf("Forward %s uses generic 'pod' resource and must have a selector", fwd.ID()),
			})
		}
	}

	// For service resources
	if resourceType == "service" {
		if len(parts) < 2 || parts[1] == "" {
			errs = append(errs, ValidationError{
				Field:   "resource",
				Message: fmt.Sprintf("Service name cannot be empty for forward %s (format: service/name)", fwd.ID()),
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

// validateSpecDurations validates duration strings in HealthCheck and Reliability specs.
func (v *Validator) validateSpecDurations(cfg *Config) []ValidationError {
	var errs []ValidationError

	// Validate HealthCheck durations
	if cfg.HealthCheck != nil {
		if cfg.HealthCheck.Interval != "" {
			if _, err := time.ParseDuration(cfg.HealthCheck.Interval); err != nil {
				errs = append(errs, ValidationError{
					Field:   "healthCheck.interval",
					Message: fmt.Sprintf("Invalid health check interval '%s': %v", cfg.HealthCheck.Interval, err),
				})
			}
		}

		if cfg.HealthCheck.Timeout != "" {
			if _, err := time.ParseDuration(cfg.HealthCheck.Timeout); err != nil {
				errs = append(errs, ValidationError{
					Field:   "healthCheck.timeout",
					Message: fmt.Sprintf("Invalid health check timeout '%s': %v", cfg.HealthCheck.Timeout, err),
				})
			}
		}

		if cfg.HealthCheck.MaxConnectionAge != "" {
			if _, err := time.ParseDuration(cfg.HealthCheck.MaxConnectionAge); err != nil {
				errs = append(errs, ValidationError{
					Field:   "healthCheck.maxConnectionAge",
					Message: fmt.Sprintf("Invalid max connection age '%s': %v", cfg.HealthCheck.MaxConnectionAge, err),
				})
			}
		}

		if cfg.HealthCheck.MaxIdleTime != "" {
			if _, err := time.ParseDuration(cfg.HealthCheck.MaxIdleTime); err != nil {
				errs = append(errs, ValidationError{
					Field:   "healthCheck.maxIdleTime",
					Message: fmt.Sprintf("Invalid max idle time '%s': %v", cfg.HealthCheck.MaxIdleTime, err),
				})
			}
		}

		// Validate health check method
		if cfg.HealthCheck.Method != "" && !isValidHealthCheckMethod(cfg.HealthCheck.Method) {
			errs = append(errs, ValidationError{
				Field:   "healthCheck.method",
				Message: fmt.Sprintf("Invalid health check method '%s' (must be one of: %s)", cfg.HealthCheck.Method, strings.Join(validHealthCheckMethods, ", ")),
			})
		}
	}

	// Validate Reliability durations
	if cfg.Reliability != nil {
		if cfg.Reliability.TCPKeepalive != "" {
			if _, err := time.ParseDuration(cfg.Reliability.TCPKeepalive); err != nil {
				errs = append(errs, ValidationError{
					Field:   "reliability.tcpKeepalive",
					Message: fmt.Sprintf("Invalid TCP keepalive duration '%s': %v", cfg.Reliability.TCPKeepalive, err),
				})
			}
		}

		if cfg.Reliability.DialTimeout != "" {
			if _, err := time.ParseDuration(cfg.Reliability.DialTimeout); err != nil {
				errs = append(errs, ValidationError{
					Field:   "reliability.dialTimeout",
					Message: fmt.Sprintf("Invalid dial timeout '%s': %v", cfg.Reliability.DialTimeout, err),
				})
			}
		}

		if cfg.Reliability.WatchdogPeriod != "" {
			if _, err := time.ParseDuration(cfg.Reliability.WatchdogPeriod); err != nil {
				errs = append(errs, ValidationError{
					Field:   "reliability.watchdogPeriod",
					Message: fmt.Sprintf("Invalid watchdog period '%s': %v", cfg.Reliability.WatchdogPeriod, err),
				})
			}
		}
	}

	return errs
}

// validateHTTPLog validates HTTP log configuration.
func (v *Validator) validateHTTPLog(fwd *Forward) []ValidationError {
	var errs []ValidationError

	if fwd.HTTPLog == nil {
		return errs
	}

	// Validate maxBodySize is non-negative
	if fwd.HTTPLog.MaxBodySize < 0 {
		errs = append(errs, ValidationError{
			Field:   "httpLog.maxBodySize",
			Message: fmt.Sprintf("Invalid maxBodySize %d for forward %s (must be non-negative)", fwd.HTTPLog.MaxBodySize, fwd.ID()),
		})
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
	if len(name) == 0 || len(name) > DNS1123LabelMaxLength {
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

// isValidResourceType returns true if the resource type is valid.
func isValidResourceType(resourceType string) bool {
	for _, rt := range validResourceTypes {
		if rt == resourceType {
			return true
		}
	}
	return false
}

// isValidHealthCheckMethod returns true if the health check method is valid.
func isValidHealthCheckMethod(method string) bool {
	for _, m := range validHealthCheckMethods {
		if m == method {
			return true
		}
	}
	return false
}

// validateContextName validates that a context name follows the allowed format.
// Context names must consist of alphanumeric characters, hyphens, or underscores,
// and must start and end with an alphanumeric character.
// This more permissive validation supports various kubeconfig naming conventions
// (e.g., "gke_project_zone_cluster", "minikube", "docker-desktop").
func validateContextName(name, field string) *ValidationError {
	if len(name) > DNS1123SubdomainMaxLength {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("Context name '%s' exceeds maximum length of %d characters", name, DNS1123SubdomainMaxLength),
		}
	}

	if !contextNameRegexp.MatchString(name) {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("Context name '%s' is not valid (must consist of alphanumeric characters, hyphens, or underscores, and start/end with alphanumeric)", name),
		}
	}

	return nil
}

// validateNamespaceName validates that a namespace name is a valid DNS subdomain (RFC 1123).
// Kubernetes namespaces must follow DNS subdomain format which allows dots for subdomain separation.
// This is more permissive than DNS labels and supports names like "kube-system", "my-app.ns".
func validateNamespaceName(name, field string) *ValidationError {
	if len(name) > DNS1123SubdomainMaxLength {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("Namespace name '%s' exceeds maximum length of %d characters", name, DNS1123SubdomainMaxLength),
		}
	}

	if !dns1123SubdomainRegexp.MatchString(name) {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("Namespace name '%s' is not a valid DNS subdomain (must consist of lowercase alphanumeric characters, '-', or '.', start with alphanumeric, end with alphanumeric)", name),
		}
	}

	return nil
}

// validateDNS1123Label validates that a name is a valid DNS label (RFC 1123).
// Used for context names and namespace names.
func validateDNS1123Label(name, field, entityType string) *ValidationError {
	if len(name) > DNS1123LabelMaxLength {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("%s name '%s' exceeds maximum length of %d characters", entityType, name, DNS1123LabelMaxLength),
		}
	}

	if !dns1123LabelRegexp.MatchString(name) {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("%s name '%s' is not a valid DNS label (must consist of lowercase alphanumeric characters or '-', start with alphanumeric, end with alphanumeric)", entityType, name),
		}
	}

	return nil
}

// validateDNS1123Subdomain validates that a name is a valid DNS subdomain name (RFC 1123).
// Used for resource names which can contain dots.
func validateDNS1123Subdomain(name, field, entityType string) *ValidationError {
	if len(name) > DNS1123SubdomainMaxLength {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("%s '%s' exceeds maximum length of %d characters", entityType, name, DNS1123SubdomainMaxLength),
		}
	}

	if !dns1123SubdomainRegexp.MatchString(name) {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("%s '%s' is not a valid DNS subdomain name (must consist of lowercase alphanumeric characters, '-', or '.', start with alphanumeric, end with alphanumeric)", entityType, name),
		}
	}

	return nil
}

// ValidatePort validates a port number and returns an error if invalid.
// This is a public function that can be used externally.
func ValidatePort(port int, name string) error {
	if !IsValidPort(port) {
		return fmt.Errorf("%s must be between %d and %d, got %d", name, MinPort, MaxPort, port)
	}
	return nil
}

// ValidateResourceFormat validates that a resource string is in the correct format.
// This is a public function that can be used externally.
func ValidateResourceFormat(resource string) error {
	parts := strings.SplitN(resource, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("resource must be in format 'type/name', got: %s", resource)
	}

	resourceType := parts[0]
	if !isValidResourceType(resourceType) {
		return fmt.Errorf("invalid resource type '%s' (must be one of: %s)", resourceType, strings.Join(validResourceTypes, ", "))
	}

	if parts[1] == "" {
		return fmt.Errorf("resource name cannot be empty in '%s'", resource)
	}

	return nil
}

// ValidateDuration validates that a string is a valid duration.
// This is a public function that can be used externally.
func ValidateDuration(duration, name string) error {
	if duration == "" {
		return nil // Empty durations are allowed (will use defaults)
	}
	_, err := time.ParseDuration(duration)
	if err != nil {
		return fmt.Errorf("invalid %s '%s': %v", name, duration, err)
	}
	return nil
}
