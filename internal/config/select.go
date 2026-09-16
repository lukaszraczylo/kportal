package config

import (
	"fmt"
	"strings"

	"github.com/lukaszraczylo/kportal/internal/logger"
)

// NormalizeContextSelection turns raw --context flag values into a clean
// selection: comma-separated entries are split, surrounding whitespace is
// trimmed, empty segments are dropped, and duplicates are removed while
// preserving first-seen order.
//
// Duplicates matter beyond tidiness: a repeated name would select the same
// context twice, producing duplicate forward IDs and spurious port conflicts.
func NormalizeContextSelection(names []string) []string {
	var (
		out  []string
		seen = make(map[string]struct{})
	)

	for _, raw := range names {
		for _, part := range strings.Split(raw, ",") {
			name := strings.TrimSpace(part)
			if name == "" {
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}

	return out
}

// contextIsActive reports whether name is part of the active selection. An
// empty selection means every context is active, which is kportal's default.
func contextIsActive(name string, activeContexts []string) bool {
	if len(activeContexts) == 0 {
		return true
	}
	for _, active := range activeContexts {
		if active == name {
			return true
		}
	}
	return false
}

// PortsCanConflict reports whether forwards living in contextA and contextB can
// ever run at the same time and therefore compete for the same local port.
//
// Two forwards collide when they share a context, or when both of their
// contexts are active simultaneously. Forwards in contexts that are never
// started together are free to reuse local ports - which is the whole point of
// selecting contexts: the same service can keep the same local port in every
// cluster.
func PortsCanConflict(contextA, contextB string, activeContexts []string) bool {
	if contextA == contextB {
		return true
	}
	return contextIsActive(contextA, activeContexts) && contextIsActive(contextB, activeContexts)
}

// SelectContexts returns a shallow copy of c containing only the named
// contexts, keeping the order they appear in the config file so the UI's row
// order stays stable regardless of the order names were passed on the command
// line. An empty selection returns c unchanged.
//
// Unknown names are an error listing the contexts that do exist, so a typo
// surfaces immediately instead of silently forwarding nothing.
//
// The returned config shares backing arrays with c and must never be written
// back to disk - use the original config for that.
func (c *Config) SelectContexts(names []string) (*Config, error) {
	selected, missing := c.selectContexts(names)
	if len(missing) > 0 {
		return nil, fmt.Errorf("unknown context %s in --context (available: %s)",
			quoteAll(missing), availableContexts(c))
	}
	return selected, nil
}

// selectContextsLenient is SelectContexts without the error: unknown names are
// dropped and returned to the caller. Reload paths use this so that renaming a
// context in the config file does not freeze hot-reload for the rest of the
// session.
func (c *Config) selectContextsLenient(names []string) (*Config, []string) {
	return c.selectContexts(names)
}

func (c *Config) selectContexts(names []string) (*Config, []string) {
	if len(names) == 0 {
		return c, nil
	}

	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = false
	}

	// Iterate the config rather than the selection so file order is preserved.
	filtered := make([]Context, 0, len(names))
	for _, ctx := range c.Contexts {
		if _, ok := wanted[ctx.Name]; !ok {
			continue
		}
		wanted[ctx.Name] = true
		filtered = append(filtered, ctx)
	}

	var missing []string
	for _, name := range names {
		if !wanted[name] {
			missing = append(missing, name)
		}
	}

	selected := *c
	selected.Contexts = filtered
	return &selected, missing
}

// availableContexts renders the context names present in the config, for use
// in error messages.
func availableContexts(c *Config) string {
	if len(c.Contexts) == 0 {
		return "none defined in the config file"
	}
	names := make([]string, 0, len(c.Contexts))
	for _, ctx := range c.Contexts {
		names = append(names, ctx.Name)
	}
	return quoteAll(names)
}

func quoteAll(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}
	return strings.Join(quoted, ", ")
}

// LoadForRuntime loads, validates and filters the configuration for the
// forward manager. Validation runs against the *whole* file - so a mistake in a
// context you are not currently forwarding is still reported - while the
// uniqueness checks that only matter for running forwards (local ports, mDNS
// hostnames) are scoped to activeContexts.
//
// Every path that re-reads the config at runtime goes through here, so the
// selection cannot be forgotten in one of them.
func LoadForRuntime(path string, activeContexts []string) (*Config, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}

	validator := NewValidator()
	if errs := validator.ValidateConfigWithOpts(cfg, ValidateOptions{
		ActiveContexts: activeContexts,
	}); len(errs) > 0 {
		return nil, fmt.Errorf("configuration is invalid:\n%s", FormatValidationErrors(errs))
	}

	selected, missing := cfg.selectContextsLenient(activeContexts)
	if len(missing) > 0 {
		logger.Info("Ignoring selected contexts that are no longer in the config file", map[string]interface{}{
			"contexts": strings.Join(missing, ", "),
		})
	}

	return selected, nil
}
