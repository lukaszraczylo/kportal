package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lukaszraczylo/kportal/internal/config"
)

// ColumnKey identifies a forwards-table column.
type ColumnKey string

// Recognized column keys, matching the names accepted in tui.columns.
const (
	ColKeyContext   ColumnKey = "context"
	ColKeyNamespace ColumnKey = "namespace"
	ColKeyAlias     ColumnKey = "alias"
	ColKeyType      ColumnKey = "type"
	ColKeyResource  ColumnKey = "resource"
	ColKeyRemote    ColumnKey = "remote"
	ColKeyLocal     ColumnKey = "local"
	ColKeyStatus    ColumnKey = "status"
)

// ResolvedColumn describes a single column ready for rendering: its header
// text, pad width, truncation limit, and identity.
type ResolvedColumn struct {
	Key    ColumnKey
	Header string
	// Width is the padded width, used by the plain table only.
	Width int
	// Max is the truncation limit; zero leaves the value untruncated.
	Max int
}

// tuiDefaultColumns is the Bubble Tea column set and order, used when the
// config does not specify tui.columns.
var tuiDefaultColumns = []ResolvedColumn{
	{Key: ColKeyContext, Header: "CONTEXT", Width: ColumnWidthContext, Max: ColumnWidthContext},
	{Key: ColKeyNamespace, Header: "NAMESPACE", Width: ColumnWidthNamespace, Max: ColumnWidthNamespace},
	{Key: ColKeyAlias, Header: "ALIAS", Width: ColumnWidthAlias, Max: ColumnWidthAlias},
	{Key: ColKeyType, Header: "TYPE", Width: ColumnWidthType, Max: ColumnWidthType},
	{Key: ColKeyResource, Header: "RESOURCE", Width: ColumnWidthResource, Max: ColumnWidthResource},
	{Key: ColKeyRemote, Header: "REMOTE"},
	{Key: ColKeyLocal, Header: "LOCAL"},
	{Key: ColKeyStatus, Header: "STATUS"},
}

// plainDefaultColumns is the -verbose table column set, widths and headers as
// rendered before columns became configurable.
var plainDefaultColumns = []ResolvedColumn{
	{Key: ColKeyContext, Header: "CONTEXT", Width: 15},
	{Key: ColKeyNamespace, Header: "NAMESPACE", Width: 18},
	{Key: ColKeyAlias, Header: "ALIAS", Width: 25, Max: 25},
	{Key: ColKeyType, Header: "TYPE", Width: 10},
	{Key: ColKeyResource, Header: "RESOURCE", Width: 25, Max: 25},
	{Key: ColKeyRemote, Header: "REMOTE PORT", Width: 12},
	{Key: ColKeyLocal, Header: "LOCAL PORT", Width: 12},
	{Key: ColKeyStatus, Header: "STATUS", Width: 12},
}

// ResolveColumns builds the Bubble Tea columns from the tui.columns
// configuration. A nil config, or one with no tui.columns entries, yields the
// built-in default column set and order.
func ResolveColumns(cfg *config.Config) []ResolvedColumn {
	return resolveColumns(cfg, tuiDefaultColumns)
}

// plainStatusWidth fits the longest status with indicator ("↻ Reconnecting"),
// so a status column that is not last cannot push later columns out of line.
const plainStatusWidth = 14

// ResolvePlainColumns is ResolveColumns for the -verbose table.
func ResolvePlainColumns(cfg *config.Config) []ResolvedColumn {
	defaults := plainDefaultColumns
	if len(cfg.GetTableColumns()) > 0 {
		defaults = slices.Clone(plainDefaultColumns)
		defaults[columnIndex(defaults, ColKeyStatus)].Width = plainStatusWidth
	}
	return resolveColumns(cfg, defaults)
}

func resolveColumns(cfg *config.Config, defaults []ResolvedColumn) []ResolvedColumn {
	var configured []config.TableColumn
	if cfg != nil {
		configured = cfg.GetTableColumns()
	}

	byKey := make(map[ColumnKey]ResolvedColumn, len(defaults))
	for _, c := range defaults {
		byKey[c.Key] = c
	}

	resolved := make([]ResolvedColumn, 0, len(configured))
	for _, c := range configured {
		col, ok := byKey[ColumnKey(strings.ToLower(strings.TrimSpace(c.Name)))]
		if !ok {
			// Unknown column names are rejected by the validator before this
			// point is reached; skip defensively rather than render garbage.
			continue
		}

		if c.Width > 0 {
			col.Width = c.Width
			col.Max = c.Width
		}

		resolved = append(resolved, col)
	}

	if len(resolved) == 0 {
		return slices.Clone(defaults)
	}

	return resolved
}

// columnValue returns the display value for the given column of a forward,
// truncated to the column's Max when set.
func columnValue(col ResolvedColumn, fwd *ForwardStatus) string {
	switch col.Key {
	case ColKeyContext:
		return clip(fwd.Context, col.Max)
	case ColKeyNamespace:
		return clip(fwd.Namespace, col.Max)
	case ColKeyAlias:
		return clip(fwd.Alias, col.Max)
	case ColKeyType:
		return clip(fwd.Type, col.Max)
	case ColKeyResource:
		return clip(fwd.Resource, col.Max)
	case ColKeyRemote:
		return fmt.Sprintf("%d", fwd.RemotePort)
	case ColKeyLocal:
		return fmt.Sprintf("%d", fwd.LocalPort)
	case ColKeyStatus:
		return fwd.Status
	default:
		return ""
	}
}

// clip truncates s to limit runes; a non-positive limit leaves it untouched.
func clip(s string, limit int) string {
	if limit <= 0 {
		return s
	}
	return truncate(s, limit)
}

// columnIndex returns the position of key within cols, or -1 if absent.
func columnIndex(cols []ResolvedColumn, key ColumnKey) int {
	for i, c := range cols {
		if c.Key == key {
			return i
		}
	}
	return -1
}
