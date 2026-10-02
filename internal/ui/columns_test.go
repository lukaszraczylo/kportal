package ui

import (
	"testing"

	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestResolveColumns_Default(t *testing.T) {
	cols := ResolveColumns(nil)
	assert.Equal(t, tuiDefaultColumns, cols)

	cols = ResolveColumns(&config.Config{})
	assert.Equal(t, tuiDefaultColumns, cols)
}

func TestResolveColumns_CustomOrderAndVisibility(t *testing.T) {
	cfg := &config.Config{
		TUI: &config.TUISpec{
			Columns: []config.TableColumn{
				{Name: "status"},
				{Name: "alias", Width: 40},
			},
		},
	}

	cols := ResolveColumns(cfg)

	assert.Len(t, cols, 2)
	assert.Equal(t, ColKeyStatus, cols[0].Key)
	assert.Equal(t, ColKeyAlias, cols[1].Key)
	assert.Equal(t, 40, cols[1].Width)
}

func TestResolveColumns_UsesDefaultWidthWhenUnset(t *testing.T) {
	cfg := &config.Config{
		TUI: &config.TUISpec{
			Columns: []config.TableColumn{
				{Name: "namespace"},
			},
		},
	}

	cols := ResolveColumns(cfg)

	assert.Len(t, cols, 1)
	assert.Equal(t, ColumnWidthNamespace, cols[0].Width)
}

func TestResolveColumns_UnknownNamesSkipped(t *testing.T) {
	cfg := &config.Config{
		TUI: &config.TUISpec{
			Columns: []config.TableColumn{
				{Name: "bogus"},
			},
		},
	}

	// All entries invalid -> fall back to defaults.
	assert.Equal(t, tuiDefaultColumns, ResolveColumns(cfg))
}

func TestColumnValue(t *testing.T) {
	fwd := &ForwardStatus{
		Context:    "prod",
		Namespace:  "default",
		Alias:      "api",
		Type:       "service",
		Resource:   "api",
		RemotePort: 8080,
		LocalPort:  8081,
		Status:     "Active",
	}

	assert.Equal(t, "prod", columnValue(ResolvedColumn{Key: ColKeyContext, Max: 10}, fwd))
	assert.Equal(t, "8080", columnValue(ResolvedColumn{Key: ColKeyRemote}, fwd))
	assert.Equal(t, "8081", columnValue(ResolvedColumn{Key: ColKeyLocal}, fwd))
	assert.Equal(t, "Active", columnValue(ResolvedColumn{Key: ColKeyStatus}, fwd))
}

func TestColumnIndex(t *testing.T) {
	cols := []ResolvedColumn{{Key: ColKeyContext}, {Key: ColKeyStatus}}
	assert.Equal(t, 1, columnIndex(cols, ColKeyStatus))
	assert.Equal(t, -1, columnIndex(cols, ColKeyLocal))
}

func TestResolveColumns_DefaultIsCopy(t *testing.T) {
	cols := ResolveColumns(nil)
	cols[0].Header = "MUTATED"

	assert.Equal(t, "CONTEXT", ResolveColumns(nil)[0].Header)
	assert.Equal(t, "CONTEXT", tuiDefaultColumns[0].Header)
}

func TestResolvePlainColumns_LegacyDefaults(t *testing.T) {
	cols := ResolvePlainColumns(nil)

	assert.Equal(t, plainDefaultColumns, cols)
	assert.Equal(t, "REMOTE PORT", cols[5].Header)
	assert.Equal(t, 15, cols[0].Width)

	custom := ResolvePlainColumns(&config.Config{TUI: &config.TUISpec{Columns: []config.TableColumn{{Name: " Status "}, {Name: "alias", Width: 7}}}})
	assert.Equal(t, plainStatusWidth, custom[0].Width)
	assert.Equal(t, 7, custom[1].Width)
	assert.Equal(t, 7, custom[1].Max)
}
