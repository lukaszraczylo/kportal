package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func typeKeys(t *testing.T, m model, text string) {
	t.Helper()
	for _, r := range text {
		m.handleAddWizardKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestNamespacesLoaded_ListFailureSwitchesToManualEntry(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)
	m.ui.addWizard.loading = true

	m.handleNamespacesLoaded(NamespacesLoadedMsg{err: errors.New("namespaces is forbidden")})

	w := m.ui.addWizard
	assert.True(t, w.namespaceManual)
	assert.Equal(t, InputModeText, w.inputMode)
	assert.NoError(t, w.error)
	assert.False(t, w.loading)
	assert.Contains(t, m.renderSelectNamespace(), "Enter Namespace")
	assert.Contains(t, m.renderSelectNamespace(), "forbidden")
}

func TestNamespacesLoaded_EmptyListSwitchesToManualEntry(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)

	m.handleNamespacesLoaded(NamespacesLoadedMsg{namespaces: []string{}})

	assert.True(t, m.ui.addWizard.namespaceManual)
}

func TestNamespacesLoaded_ListedNamespacesKeepSelectionMode(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)

	m.handleNamespacesLoaded(NamespacesLoadedMsg{namespaces: []string{"default", "kube-system"}})

	w := m.ui.addWizard
	assert.False(t, w.namespaceManual)
	assert.Equal(t, InputModeList, w.inputMode)
	assert.Equal(t, []string{"default", "kube-system"}, w.namespaces)
}

func TestManualNamespace_TypeAndConfirm(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)
	m.handleNamespacesLoaded(NamespacesLoadedMsg{err: errors.New("forbidden")})

	// k and j must be typed, not treated as navigation
	typeKeys(t, m, "kube-jack")
	assert.Equal(t, "kube-jack", m.ui.addWizard.textInput)

	m.handleAddWizardEnter()

	w := m.ui.addWizard
	assert.Equal(t, "kube-jack", w.selectedNamespace)
	assert.Equal(t, StepSelectResourceType, w.step)
	assert.Equal(t, InputModeList, w.inputMode)
	assert.Empty(t, w.textInput)
}

func TestManualNamespace_RejectsInvalidName(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)
	m.handleNamespacesLoaded(NamespacesLoadedMsg{err: errors.New("forbidden")})
	m.ui.addWizard.selectedNamespace = ""

	for _, bad := range []string{"", "Has_Upper", "-leading", "trailing-"} {
		m.ui.addWizard.textInput = bad
		m.handleAddWizardEnter()

		w := m.ui.addWizard
		require.Error(t, w.error, "name %q", bad)
		assert.Equal(t, StepSelectNamespace, w.step, "name %q", bad)
		assert.Empty(t, w.selectedNamespace, "name %q", bad)
	}
}

func TestManualNamespace_BackFromLaterStepRestoresTextInput(t *testing.T) {
	m := newModelWithWizard(StepSelectResourceType)
	m.ui.addWizard.namespaceManual = true

	m.handleAddWizardKeys(tea.KeyMsg{Type: tea.KeyEsc})

	w := m.ui.addWizard
	assert.Equal(t, StepSelectNamespace, w.step)
	assert.Equal(t, InputModeText, w.inputMode)
}

func TestKeysJK_StillNavigateInListMode(t *testing.T) {
	m := newModelWithWizard(StepSelectResourceType)
	m.ui.addWizard.inputMode = InputModeList

	m.handleAddWizardKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	assert.Equal(t, 1, m.ui.addWizard.cursor)
}
