package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

const commandPaletteMaxVisible = 12

type commandPaletteKind int

const (
	commandPaletteActions commandPaletteKind = iota
	commandPaletteModel
	commandPaletteAgent
	commandPaletteWorkflow
	commandPaletteUtilityModel
	commandPaletteReviewerModel
)

type utilityModelSelectionResult struct {
	Ref provider.ModelRef
}

type reviewerModelSelectionResult struct {
	Ref provider.ModelRef
}

type workflowSelectionResult struct {
	WorkflowID string
}

type modelCatalogRefreshRequestedMsg struct {
	query    string
	selected provider.ModelRef
}

type commandPaletteActionsItems struct {
	ModelItems            []modelItem
	CurrentUtilityModel   string
	CurrentReviewerModel  string
	AllowMutableSelection bool
}

type commandPaletteAction struct {
	ID          string
	Title       string
	Description string
}

type commandPaletteActionResult struct {
	ActionID string
}

type commandPaletteListOption struct {
	Label       string
	Description string
	Disabled    bool
	Agent       agentItem
	Workflow    workflowItem
	Model       modelItem
	Action      commandPaletteAction
}

type paletteButton struct {
	id    string
	label string
}

type commandPaletteDialog struct {
	id              string
	kind            commandPaletteKind
	returnToActions bool
	frameWidth      int
	frameHeight     int
	theme           *theme.Theme

	paletteListState

	filter textinput.Model

	actions            []commandPaletteAction
	agentItems         []agentItem
	workflowItems      []workflowItem
	modelItems         []modelItem
	currentAgent       string
	currentWorkflow    string
	currentModel       string
	allowMutableSelect bool
}

func newCommandPaletteActions(items commandPaletteActionsItems, th *theme.Theme) *commandPaletteDialog {
	dialog := newCommandPaletteDialog(dialogIDCommandPalette, commandPaletteActions, th)
	dialog.returnToActions = true
	dialog.allowMutableSelect = items.AllowMutableSelection
	dialog.actions = []commandPaletteAction{
		{ID: "select-model", Title: "Switch model", Description: "Choose the primary model for this session"},
		{ID: "select-agent", Title: "Switch agent", Description: "Choose the active agent"},
		{ID: "select-workflow", Title: "Select workflow", Description: "Choose a workflow for the next turn"},
		{ID: "select-theme", Title: "Switch theme", Description: "Choose the active theme"},
		{ID: "timeline", Title: "Timeline", Description: "Branch from a previous turn"},
		{ID: "manage-trust", Title: "Manage trust", Description: "Review and revoke workspace or MCP trust"},
		{ID: "new-session", Title: "New session", Description: "Start a fresh workspace session"},
		{ID: "connect-provider", Title: "Connect provider", Description: "Add, update, or remove model providers"},
		{
			ID:          "select-utility-model",
			Title:       "Select utility model",
			Description: "Set the global utility model. " + utilityModelStatusSummary(items.CurrentUtilityModel, items.ModelItems),
		},
	}
	if strings.TrimSpace(items.CurrentUtilityModel) != "" {
		dialog.actions = append(dialog.actions, commandPaletteAction{
			ID:          "unset-utility-model",
			Title:       "Unset utility model",
			Description: "Use the primary model fallback for utility tasks",
		})
	}
	dialog.actions = append(dialog.actions, commandPaletteAction{
		ID:          "select-reviewer-model",
		Title:       "Select reviewer model",
		Description: "Set the global reviewer model. " + reviewerModelStatusSummary(items.CurrentReviewerModel, items.ModelItems),
	})
	if strings.TrimSpace(items.CurrentReviewerModel) != "" {
		dialog.actions = append(dialog.actions, commandPaletteAction{
			ID:          "unset-reviewer-model",
			Title:       "Unset reviewer model",
			Description: "Use the session primary model fallback for review turns",
		})
	}
	dialog.modelItems = append([]modelItem(nil), items.ModelItems...)
	return dialog
}

func newModelDialog(modelItems []modelItem, currentModel string, allowMutableSelection bool, th *theme.Theme) *commandPaletteDialog {
	dialog := newCommandPaletteDialog(dialogIDCommandPalette, commandPaletteModel, th)
	dialog.currentModel = currentModel
	dialog.allowMutableSelect = allowMutableSelection
	dialog.modelItems = append([]modelItem(nil), modelItems...)
	return dialog
}

func newAgentDialog(agentItems []agentItem, currentAgent string, allowMutableSelection bool, th *theme.Theme) *commandPaletteDialog {
	dialog := newCommandPaletteDialog(dialogIDCommandPalette, commandPaletteAgent, th)
	dialog.currentAgent = currentAgent
	dialog.allowMutableSelect = allowMutableSelection
	dialog.agentItems = append([]agentItem(nil), agentItems...)
	return dialog
}

func newWorkflowDialog(workflowItems []workflowItem, currentWorkflow string, allowMutableSelection bool, th *theme.Theme) *commandPaletteDialog {
	dialog := newCommandPaletteDialog(dialogIDCommandPalette, commandPaletteWorkflow, th)
	dialog.currentWorkflow = strings.TrimSpace(currentWorkflow)
	dialog.allowMutableSelect = allowMutableSelection
	dialog.workflowItems = append([]workflowItem(nil), workflowItems...)
	return dialog
}

func newUtilityModelDialog(modelItems []modelItem, currentModel string, allowMutableSelection bool, th *theme.Theme) *commandPaletteDialog {
	dialog := newCommandPaletteDialog(dialogIDCommandPalette, commandPaletteUtilityModel, th)
	dialog.currentModel = currentModel
	dialog.allowMutableSelect = allowMutableSelection
	dialog.modelItems = append([]modelItem(nil), modelItems...)
	return dialog
}

func newReviewerModelDialog(modelItems []modelItem, currentModel string, allowMutableSelection bool, th *theme.Theme) *commandPaletteDialog {
	dialog := newCommandPaletteDialog(dialogIDCommandPalette, commandPaletteReviewerModel, th)
	dialog.currentModel = currentModel
	dialog.allowMutableSelect = allowMutableSelection
	dialog.modelItems = append([]modelItem(nil), modelItems...)
	return dialog
}

func utilityModelStatusSummary(current string, items []modelItem) string {
	return modelStatusSummary(current, items, "primary model fallback")
}

func reviewerModelStatusSummary(current string, items []modelItem) string {
	return modelStatusSummary(current, items, "session primary model fallback")
}

func modelStatusSummary(current string, items []modelItem, fallback string) string {
	current = strings.TrimSpace(current)
	if current == "" {
		return "Current: " + fallback
	}
	for _, item := range items {
		if item.Ref.String() != current {
			continue
		}
		return "Current: " + item.ProviderName + " / " + item.ModelName
	}
	if ref, err := provider.ParseModelRef(current); err == nil {
		return "Current: " + providerDisplayName(ref.ProviderID) + " / " + ref.ModelID
	}
	return "Current: " + current
}

func newCommandPaletteDialog(id string, kind commandPaletteKind, th *theme.Theme) *commandPaletteDialog {
	filter := newDialogTextInput(th, 128)
	filter.Focus()

	dialog := &commandPaletteDialog{
		id:               id,
		kind:             kind,
		frameWidth:       96,
		frameHeight:      32,
		theme:            th,
		paletteListState: newPaletteListState(commandPaletteMaxVisible),
		filter:           filter,
	}
	dialog.configureInputs()
	dialog.syncFocus()
	return dialog
}

func (d *commandPaletteDialog) ID() string { return d.id }

func (d *commandPaletteDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	applyDialogInputTheme(&d.filter, th)
}

func (d *commandPaletteDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	d.resizeInputs()
}
