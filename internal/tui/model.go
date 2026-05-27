package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

var ErrEventStreamClosed = errors.New("tui event stream closed unexpectedly")
var ErrContextRequired = errors.New("tui context is required")
var ErrTranscriptIncrementalRefreshInvariant = errors.New("incremental transcript refresh requires a cached compatible layout")

type ModelConfig struct {
	Context            context.Context
	Theme              *theme.Theme
	ThemeName          string
	Layout             string
	TerminalIcons      string
	HideShellToolCalls bool
	DisplayTurns       int
	SessionID          string
	TurnID             string
	WorkspaceRoot      string
	UserText           string
	AgentID            string
	SkillIDs           []string
	InitialState       *events.SessionState
	InitialStateOwned  bool
}

type Model struct {
	backend               Backend
	controller            controller
	clipboard             clipboardWriter
	editor                externalEditorLauncher
	ctx                   context.Context
	theme                 *theme.Theme
	themeName             string
	themeRenderTheme      *theme.Theme
	themeRenderKey        string
	layout                string
	terminalIcons         terminalIconProfile
	shellToolCallsVisible bool
	displayTurns          int
	sessionID             string
	turnID                string
	userText              string
	agentID               string
	skillIDs              []string
	thinkingEnabled       bool
	reasoningVariant      string
	workspace             string
	bootstrapped          bool
	providerCatalog       providerCatalogState

	projector *events.Projector
	stream    <-chan events.Event
	watchID   int
	nextWatch int
	cancel    context.CancelFunc

	width              int
	height             int
	busy               bool
	animation          animationState
	interaction        interactionState
	sessionNavigation  sessionNavigationState
	holdOpen           bool
	startTurn          bool
	liveTurn           liveTurnState
	selection          selectionState
	chrome             chromeState
	composer           textarea.Model
	composerState      composerState
	renderCache        renderCacheState
	sessionsBody       Messages
	messages           Messages
	mouseRegions       inputMouseRegions
	inputTrace         *inputTraceLogger
	dialogRefresh      dialogRefreshState
	transcriptRefresh  transcriptRefreshState
	transcriptView     transcriptViewState
	toolHydration      toolHydrationState
	delegatedSnapshots delegatedSnapshotState
	footerStatus       footerStatusState
	inspector          inspectorState
	dialog             dialogModel
	footerNotice       footerNoticeState
	shuttingDown       bool
	err                error
}

type dialogRefreshState struct {
	id       string
	deferred bool
	pending  bool
	ticking  bool
	lastAt   time.Time
}

type transcriptRefreshState struct {
	deferred bool
	pending  bool
	ticking  bool
	plan     transcriptRefreshPlan
	lastAt   time.Time
}

type transcriptViewState struct {
	layout             transcriptLayout
	turnSourceKeys     map[string]string
	toolLines          map[sessionToolCallRef]int
	selectionLines     []transcriptSelectionLine
	cursorLine         int
	cursorColumn       int
	cursorGoalColumn   int
	cursorInitialized  bool
	mouseSelecting     bool
	mouseAnchorLine    int
	mouseAnchorColumn  int
	visualActive       bool
	visualAnchorLine   int
	visualAnchorColumn int
}

type composerState struct {
	pendingAttachments   []Attachment
	nextAttachmentID     int
	pastedText           []composerPastedTextChunk
	nextPastedTextID     int
	promptHistory        []app.PromptHistoryEntry
	promptHistoryPending []app.PromptHistoryEntry
	promptHistoryLoaded  bool
	promptHistoryBusy    bool
	skills               []app.AvailableSkill
	skillsLoaded         bool
	skillsBusy           bool
	workspacePaths       []app.WorkspacePath
	workspacePathsLoaded bool
	workspacePathsBusy   bool
	pendingFocusPaths    []FocusPath
	nextFocusPathID      int
	err                  string
	historyRecallActive  bool
	historyRecallDraft   string
	historyRecallIndex   int
	popupMode            composerPopupMode
	popupCursor          int
}

type inspectorState struct {
	tab                 int
	body                Messages
	toolLines           map[int]inspectorToolLineAction
	taskLines           map[int]string
	collapsedToolGroups map[string]bool
	key                 string
}

type renderCacheState struct {
	rootSurface         *surfaceBufferCache
	transcriptPane      *renderedTextCache
	splitTranscriptPane *renderedTextCache
	splitInspectorPane  *renderedTextCache
	splitWideView       *renderedTextCache
	dialogOverlay       *renderedOverlayCache
	composerOverlay     *renderedOverlayCache
	transcriptMarkdown  *streamingMarkdownSurfaceCache
}

type toolHydrationState struct {
	loadedResults    map[scopedToolCallKey]app.ToolResultDetail
	loadingResults   map[scopedToolCallKey]bool
	loadedMutations  map[scopedToolCallKey]loadedToolMutationDetail
	loadingMutations map[scopedToolCallKey]bool
}

type delegatedSnapshotState struct {
	snapshots map[string]events.SessionState
	loading   map[string]bool
	pending   map[string]bool
}

type footerStatusState struct {
	workspace        app.WorkspaceStatus
	workspaceLoading bool
	budget           app.BudgetStatus
	sessionUsage     app.SessionUsageSummary
}

type providerCatalogState struct {
	agents              map[string]app.AvailableAgent
	models              map[string]app.AvailableModel
	connectedProviders  map[string]app.ConnectedProvider
	providerConfigKnown bool
	defaultModelRoute   provider.ModelRoute
	utilityModel        provider.ModelRef
}

type footerNoticeState struct {
	err      string
	activity *footerActivityState
	nextID   int
}

type sessionNavigationState struct {
	parentSessionID string
	parentHandoffID string
	viewStack       []sessionView
}

type interactionState struct {
	resolveReq     string
	resolveHandoff string
	cursor         int
}

type animationState struct {
	frame   int
	ticking bool
}

type chromeState struct {
	focus           focusRegion
	inspectorOpen   bool
	wideSidebarOpen bool
	hintsExpanded   bool
}

type selectionState struct {
	detailTurnID  string
	callSessionID string
	callTurnID    string
	callID        string
	taskID        string
	handoffID     string
}

type liveTurnState struct {
	spinnerArmed    bool
	startedAt       time.Time
	cancelRequested bool
}

type sessionView struct {
	SessionID             string
	TurnID                string
	UserText              string
	AgentID               string
	SkillIDs              []string
	ThinkingEnabled       bool
	ReasoningVariant      string
	WorkspaceRoot         string
	ParentSessionID       string
	ParentHandoffID       string
	DetailTurnID          string
	SelectedCallSessionID string
	SelectedCallTurnID    string
	SelectedCallID        string
	SelectedTaskID        string
	SelectedHandoffID     string
	Focus                 focusRegion
	InspectorOpen         bool
	WideSidebarOpen       bool
}

func NewModel(backend Backend, cfg ModelConfig) Model {
	activeTheme := cfg.Theme
	if activeTheme == nil {
		defaultTheme := theme.StaticDefault()
		activeTheme = &defaultTheme
	}
	composer := newComposer(activeTheme)
	sessionsBody := NewMessagesWithTone(activeTheme, "panel")
	messages := NewMessages(activeTheme)
	inspectorBody := NewMessagesWithTone(activeTheme, "panel")
	projector := events.NewProjector(cfg.SessionID)
	workspace := cfg.WorkspaceRoot
	userText := cfg.UserText
	agentID := strings.TrimSpace(cfg.AgentID)
	skillIDs := append([]string(nil), cfg.SkillIDs...)
	bootstrapped := false
	if cfg.InitialState != nil {
		if cfg.InitialStateOwned {
			projector = events.NewProjectorFromOwnedSnapshot(*cfg.InitialState)
		} else {
			projector = events.NewProjectorFromSnapshot(*cfg.InitialState)
		}
		workspace = resolvedWorkspaceRoot(*cfg.InitialState, cfg.WorkspaceRoot)
		userText = resolvedUserText(*cfg.InitialState, sessionView{TurnID: cfg.TurnID, UserText: cfg.UserText})
		agentID = resolvedAgentID(*cfg.InitialState, cfg.TurnID, agentID)
		if len(skillIDs) == 0 {
			skillIDs = resolvedSkillIDs(*cfg.InitialState, cfg.TurnID, skillIDs)
		}
		bootstrapped = true
	}
	thinkingEnabled := false
	reasoningVariant := ""
	if cfg.InitialState != nil {
		thinkingEnabled = resolvedThinkingEnabled(*cfg.InitialState, cfg.TurnID, thinkingEnabled)
		reasoningVariant = resolvedReasoningVariant(*cfg.InitialState, cfg.TurnID, reasoningVariant)
	}

	model := Model{
		backend:               backend,
		controller:            backend,
		clipboard:             systemClipboardWriter{},
		editor:                systemExternalEditorLauncher{},
		ctx:                   cfg.Context,
		theme:                 activeTheme,
		themeName:             normalizedThemeSelection(cfg.ThemeName),
		themeRenderTheme:      activeTheme,
		themeRenderKey:        renderThemeCacheKey(activeTheme),
		layout:                normalizedTUILayoutSelection(cfg.Layout),
		terminalIcons:         terminalIconProfileForMode(cfg.TerminalIcons),
		shellToolCallsVisible: !cfg.HideShellToolCalls,
		displayTurns:          max(cfg.DisplayTurns, 0),
		sessionID:             cfg.SessionID,
		turnID:                cfg.TurnID,
		userText:              userText,
		agentID:               pickFirstNonBlank(agentID, "builder"),
		skillIDs:              skillIDs,
		thinkingEnabled:       thinkingEnabled,
		reasoningVariant:      reasoningVariant,
		workspace:             workspace,
		bootstrapped:          bootstrapped,
		providerCatalog: providerCatalogState{
			agents:             make(map[string]app.AvailableAgent),
			connectedProviders: make(map[string]app.ConnectedProvider),
		},
		projector: projector,
		selection: selectionState{
			detailTurnID: cfg.TurnID,
		},
		chrome: chromeState{
			focus:           focusComposer,
			inspectorOpen:   true,
			wideSidebarOpen: true,
		},
		holdOpen:  true,
		nextWatch: 1,
		startTurn: true,
		composer:  composer,
		renderCache: renderCacheState{
			rootSurface:         newSurfaceBufferCache("root_surface"),
			transcriptPane:      newRenderedTextCache("transcript_pane"),
			splitTranscriptPane: newRenderedTextCache("split_transcript_pane"),
			splitInspectorPane:  newRenderedTextCache("split_inspector_pane"),
			splitWideView:       newRenderedTextCache("split_wide_view"),
			dialogOverlay:       newRenderedOverlayCache(),
			composerOverlay:     newRenderedOverlayCache(),
			transcriptMarkdown:  newStreamingMarkdownSurfaceCache(64),
		},
		sessionsBody: sessionsBody,
		messages:     messages,
		inspector: inspectorState{
			body:                inspectorBody,
			toolLines:           make(map[int]inspectorToolLineAction),
			taskLines:           make(map[int]string),
			collapsedToolGroups: make(map[string]bool),
		},
		transcriptView: transcriptViewState{
			turnSourceKeys: make(map[string]string),
		},
		toolHydration: toolHydrationState{
			loadedResults:    make(map[scopedToolCallKey]app.ToolResultDetail),
			loadingResults:   make(map[scopedToolCallKey]bool),
			loadedMutations:  make(map[scopedToolCallKey]loadedToolMutationDetail),
			loadingMutations: make(map[scopedToolCallKey]bool),
		},
		delegatedSnapshots: delegatedSnapshotState{
			snapshots: make(map[string]events.SessionState),
			loading:   make(map[string]bool),
			pending:   make(map[string]bool),
		},
	}
	if cfg.InitialState != nil {
		model.primeTranscriptTurnSourceKeys(*cfg.InitialState)
	}
	_ = model.composer.Focus()
	model.syncComposerPrompt()
	if cfg.Context == nil {
		model.err = ErrContextRequired
	}
	if backend == nil {
		model.err = ErrBackendRequired
	}
	if model.err == nil {
		_ = model.refreshAvailableAgents()
		_ = model.refreshDialogState()
		model.syncFocusState()
		_ = model.syncComposerFocus()
	}
	return model
}

func (m Model) Err() error {
	return m.err
}

func (m Model) WithStartTurn(start bool) Model {
	m.startTurn = start
	return m
}

func (m Model) Init() tea.Cmd {
	if m.err != nil {
		return tea.Quit
	}
	cmds := make([]tea.Cmd, 0, 2)
	if m.shouldAutoOpenProviderDialog() {
		cmds = append(cmds, m.openConnectDialog())
	}
	if strings.TrimSpace(m.sessionID) == "" {
		return tea.Batch(cmds...)
	}
	if m.bootstrapped {
		cmds = append(cmds, watchSessionCmd(m.ctx, m.controller, m.sessionID, m.projector.Snapshot().LastSequence, m.startTurn, m.nextWatch))
		return tea.Batch(cmds...)
	}
	cmds = append(cmds, openSessionCmd(m.ctx, m.controller, m.currentView(), m.startTurn, m.nextWatch))
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (next tea.Model, cmd tea.Cmd) {
	start := time.Now()
	defer func() {
		traceTUILoop("model_update", msg, time.Since(start), 0)
	}()
	if next, cmd, handled := m.updateChromeMsg(msg); handled {
		return next, cmd
	}
	if next, cmd, handled := m.updateSessionLifecycleMsg(msg); handled {
		return next, cmd
	}
	if next, cmd, handled := m.updateAsyncStateMsg(msg); handled {
		return next, cmd
	}
	if next, cmd, handled := m.updateInputAndTickMsg(msg); handled {
		return next, cmd
	}
	if m.dialog != nil {
		updated, cmd := m.dialog.Update(msg)
		m.dialog = updated
		return m, tea.Batch(cmd, m.syncDeferredDialogIfNeeded())
	}
	return m, nil
}

func (m Model) View() tea.View {
	start := time.Now()
	content, cursor := renderModelSurface(m)
	traceTUILoop("model_view", nil, time.Since(start), len(content))
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.Cursor = cursor
	if bg := toneValue(m.theme, toneBG); bg != "" {
		view.BackgroundColor = lipgloss.Color(bg)
	}
	if m.shuttingDown {
		view.WindowTitle = ""
	} else {
		view.WindowTitle = m.windowTitle()
	}
	return view
}
