package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/mocks"
	"github.com/umputun/revdiff/app/ui/overlay"
)

// descriptionAnnotationModel builds a model whose info popup carries a description,
// with the given editor wired in. Used to exercise the issue #281 flow.
func descriptionAnnotationModel(t *testing.T, editor ExternalEditor, note string) Model {
	t.Helper()
	store := annotation.NewStore()
	if note != "" {
		store.Add(annotation.Annotation{File: annotation.DescriptionFile, Line: 0, Type: "", Comment: note})
	}
	m := testNewModel(t, &mocks.RendererMock{}, store, noopHighlighter(), ModelConfig{
		ReviewInfo: &ReviewInfoConfig{Description: "agent prose"},
	})
	m.editor = editor
	return m
}

func TestDescriptionAnnotation_BuildInfoSpecCarriesSanitizedAnnotation(t *testing.T) {
	m := descriptionAnnotationModel(t, mockEditor("", nil), "wrong \x1b[31massumption")
	spec := m.buildInfoSpec()
	assert.Equal(t, "wrong assumption", spec.DescriptionAnnotation, "note must be sanitized before display")
}

func TestOpenDescriptionEditor_SeedsFromExistingAnnotation(t *testing.T) {
	fake := mockEditor("", nil)
	m := descriptionAnnotationModel(t, fake, "prior note")
	cmd := m.openDescriptionEditor()
	require.NotNil(t, cmd)
	require.Len(t, fake.CommandCalls(), 1)
	assert.Equal(t, "prior note", fake.CommandCalls()[0].Content, "editor must be seeded with the existing note")
}

func TestOpenDescriptionEditor_SeedsFromDescriptionOnFirstEdit(t *testing.T) {
	fake := mockEditor("", nil)
	m := descriptionAnnotationModel(t, fake, "") // no existing note
	cmd := m.openDescriptionEditor()
	require.NotNil(t, cmd)
	require.Len(t, fake.CommandCalls(), 1)
	assert.Equal(t, "agent prose", fake.CommandCalls()[0].Content,
		"first edit must seed from the description prose, not a blank file")
}

func TestOpenDescriptionEditor_CommandErrorProducesFinishedMsg(t *testing.T) {
	cmdErr := errors.New("temp file unavailable")
	m := descriptionAnnotationModel(t, mockEditor("", cmdErr), "seed note")
	cmd := m.openDescriptionEditor()
	require.NotNil(t, cmd)
	msg, ok := cmd().(descriptionEditorFinishedMsg)
	require.True(t, ok, "expected descriptionEditorFinishedMsg, got %T", cmd())
	assert.Equal(t, cmdErr, msg.err)
	assert.Equal(t, "seed note", msg.seed)
}

func TestHandleDescriptionEditorFinished_StoresAnnotationAndReopensPopup(t *testing.T) {
	m := descriptionAnnotationModel(t, mockEditor("", nil), "")
	result, _ := m.Update(descriptionEditorFinishedMsg{content: "correcting the intent"})
	model := result.(Model)

	got := model.store.Get(annotation.DescriptionFile)
	require.Len(t, got, 1)
	assert.Equal(t, 0, got[0].Line, "annotation is stored file-level")
	assert.Equal(t, "correcting the intent", got[0].Comment)
	assert.Equal(t, overlay.KindInfo, model.overlay.Kind(), "info popup reopens so the note shows inline")
}

func TestHandleDescriptionEditorFinished_EmptyContentClearsAnnotation(t *testing.T) {
	m := descriptionAnnotationModel(t, mockEditor("", nil), "existing note")
	result, _ := m.Update(descriptionEditorFinishedMsg{content: ""})
	model := result.(Model)
	assert.Empty(t, model.store.Get(annotation.DescriptionFile), "emptying the editor deletes the note")
}

func TestHandleDescriptionEditorFinished_ErrorWithUnchangedSeedKeepsAnnotation(t *testing.T) {
	m := descriptionAnnotationModel(t, mockEditor("", nil), "keep me")
	result, _ := m.Update(descriptionEditorFinishedMsg{
		content: "keep me",
		seed:    "keep me",
		err:     errors.New("tty release failed"),
	})
	model := result.(Model)
	got := model.store.Get(annotation.DescriptionFile)
	require.Len(t, got, 1, "unchanged read-back on error is treated as no edit — note preserved")
	assert.Equal(t, "keep me", got[0].Comment)
}

func TestHandleDescriptionEditorFinished_RestoreMouseEmitsEnable(t *testing.T) {
	m := descriptionAnnotationModel(t, mockEditor("", nil), "")
	_, cmd := m.Update(descriptionEditorFinishedMsg{content: "note", restoreMouse: true})
	require.NotNil(t, cmd)
	assert.IsType(t, tea.EnableMouseCellMotion(), cmd())
}

func TestDescriptionAnnotation_OutputEmitsFileLevelBlock(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: annotation.DescriptionFile, Line: 0, Type: "", Comment: "intent note"})
	out := store.FormatOutput()
	assert.Contains(t, out, "## (description) (file-level)\nintent note")
}

// the --annotations half of the round-trip, where the synthetic key has no diff
// to resolve against, is covered by TestPreloadAnnotations_KeepsDescriptionNote.
func TestDescriptionAnnotation_RoundTripsThroughParse(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: annotation.DescriptionFile, Line: 0, Type: "", Comment: "intent note"})
	out := store.FormatOutput()

	reloaded := annotation.NewStore()
	require.NoError(t, reloaded.Load(strings.NewReader(out)))
	got := reloaded.Get(annotation.DescriptionFile)
	require.Len(t, got, 1, "the synthetic (description) key must survive the format/parse pair")
	assert.Equal(t, "intent note", got[0].Comment)
}

// the hint names a rebindable action, so it has to follow the keymap rather
// than a literal key.
func TestDescriptionAnnotation_HintFollowsTheKeymap(t *testing.T) {
	m := descriptionAnnotationModel(t, mockEditor("", nil), "")
	assert.Equal(t, "press e to annotate the description", m.buildInfoSpec().DescriptionHint, "default binding is advertised")

	m.keymap.Unbind("e")
	m.keymap.Bind("ctrl+x", keymap.ActionOpenFileInEditor)
	assert.Equal(t, "press Ctrl+X to annotate the description", m.buildInfoSpec().DescriptionHint, "a rebound action is advertised under its new key")

	m.keymap.Unbind("ctrl+x")
	assert.Empty(t, m.buildInfoSpec().DescriptionHint, "an unbound action advertises no key")
}

func TestInfoOverlay_PressingAnnotateKeyLaunchesEditor(t *testing.T) {
	fake := mockEditor("", nil)
	m := descriptionAnnotationModel(t, fake, "")
	m.overlay.OpenInfo(m.buildInfoSpec())
	require.True(t, m.overlay.Active())

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model := result.(Model)
	assert.False(t, model.overlay.Active(), "info popup closes before the editor takes the terminal")
	require.NotNil(t, cmd, "the annotate key must launch the description editor")
}
