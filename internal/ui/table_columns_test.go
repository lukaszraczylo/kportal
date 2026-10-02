package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func columnsConfig(cols ...config.TableColumn) *config.Config {
	return &config.Config{TUI: &config.TUISpec{Columns: cols}}
}

func newPlainTestTable(cfg *config.Config) *TableUI {
	tbl := NewTableUI(true, cfg)
	tbl.forwards["a"] = &ForwardStatus{
		Context: "prod-cluster-with-a-long-name", Namespace: "default", Alias: "api",
		Type: "service", Resource: "api-server", RemotePort: 8080, LocalPort: 18080, Status: "Active",
	}
	tbl.forwards["b"] = &ForwardStatus{
		Context: "dev", Namespace: "kube-system", Alias: "db", Type: "pod",
		Resource: "postgres-0", RemotePort: 5432, LocalPort: 15432, Status: "Reconnecting",
	}
	return tbl
}

func TestTableUI_Render_DefaultMatchesLegacyLayout(t *testing.T) {
	tbl := newPlainTestTable(nil)

	got := captureStdout(t, tbl.Render)

	legacyHeader := fmt.Sprintf("%-15s %-18s %-25s %-10s %-25s %-12s %-12s %-12s",
		"CONTEXT", "NAMESPACE", "ALIAS", "TYPE", "RESOURCE", "REMOTE PORT", "LOCAL PORT", "STATUS")
	legacyRow := func(f *ForwardStatus, status string) string {
		return fmt.Sprintf("  %-15s %-18s %-25s %-10s %-25s %-12d %-12d %s",
			f.Context, f.Namespace, truncate(f.Alias, 25), f.Type, truncate(f.Resource, 25), f.RemotePort, f.LocalPort, status)
	}
	want := "kportal - Port Forwarding Status\n" +
		strings.Repeat("=", 130) + "\n" +
		legacyHeader + "\n" +
		strings.Repeat("-", 130) + "\n" +
		legacyRow(tbl.forwards["b"], "↻ Reconnecting") + "\n" +
		legacyRow(tbl.forwards["a"], "✓ Active") + "\n" +
		strings.Repeat("=", 130) + "\n" +
		"Total forwards: 2 | Press Ctrl+C to stop\n\n"

	assert.Equal(t, want, got)
}

func TestPlainRows_CustomOrderStaysAligned(t *testing.T) {
	cols := ResolvePlainColumns(columnsConfig(
		config.TableColumn{Name: "alias"},
		config.TableColumn{Name: "status"},
		config.TableColumn{Name: "local"},
		config.TableColumn{Name: "remote"},
	))
	rows := []*ForwardStatus{
		{Alias: "api", Status: "Active", LocalPort: 18080, RemotePort: 80},
		{Alias: "a-much-longer-alias", Status: "Reconnecting", LocalPort: 1, RemotePort: 5432},
		{Alias: "db", Status: "Error", LocalPort: 15432, RemotePort: 5432},
	}

	// Prefix width up to the last column must match the header for every row.
	lead := visibleWidth(renderPlainHeaderRow(cols[:len(cols)-1])) + 1
	for _, f := range rows {
		line := renderPlainDataRow(cols, f)
		last := strings.LastIndex(line, " ") + 1
		assert.Equal(t, lead+2, visibleWidth(line[:last]), "row %q misaligned", line)
	}
}

func TestPadVisible_IgnoresColourCodes(t *testing.T) {
	coloured := "\033[32m●\033[0m Active"
	assert.Equal(t, 8, visibleWidth(coloured))
	assert.Equal(t, 12, visibleWidth(padVisible(coloured, 12)))
	assert.Equal(t, "toolong", padVisible("toolong", 3))
}

func TestPlainRule_MatchesColumnWidthSum(t *testing.T) {
	cols := ResolvePlainColumns(columnsConfig(
		config.TableColumn{Name: "alias", Width: 20},
		config.TableColumn{Name: "status"},
		config.TableColumn{Name: "local"},
	))

	assert.Equal(t, strings.Repeat("=", 20+plainStatusWidth+12+2), plainRule("=", cols))
	assert.Equal(t, strings.Repeat("-", 130), plainRule("-", ResolvePlainColumns(nil)))
}

func TestTableUI_SetColumns_ChangesRendering(t *testing.T) {
	tbl := newPlainTestTable(nil)
	before := captureStdout(t, tbl.Render)

	tbl.SetColumns(columnsConfig(config.TableColumn{Name: "status"}, config.TableColumn{Name: "alias"}))
	after := captureStdout(t, tbl.Render)

	assert.Contains(t, before, "CONTEXT")
	assert.NotContains(t, after, "CONTEXT")
	assert.True(t, strings.Contains(after, "STATUS         ALIAS"), after)
	assert.Contains(t, after, strings.Repeat("=", plainStatusWidth+25+1)+"\n")
}

func TestBubbleTeaUI_SetColumns_ChangesRows(t *testing.T) {
	m := newTestModelWithForward()
	m.ui.mu.RLock()
	before := m.buildTableRows()
	m.ui.mu.RUnlock()
	require.Len(t, before, 1)
	assert.Len(t, before[0], 8)

	m.ui.SetColumns(columnsConfig(config.TableColumn{Name: "alias"}, config.TableColumn{Name: "remote"}))

	m.ui.mu.RLock()
	after := m.buildTableRows()
	m.ui.mu.RUnlock()
	require.Len(t, after, 1)
	assert.Equal(t, []string{"my-app", "8080"}, after[0])
}
