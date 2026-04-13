package tui

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/lsp"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/tool"
	"github.com/sageil/kodacode/v1/internal/tui/termpalette"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

type route int

const (
	routeHome    route = iota
	routeSession route = iota
)

const (
	dialogIDAgent        = "agent"
	dialogIDSession      = "session"
	dialogIDModel        = "model"
	dialogIDPermission   = "permission"
	dialogIDTheme        = "theme"
	dialogIDUserQuestion = "user_question"
	dialogIDHelp         = "help"
	dialogIDProvider     = "provider"
	dialogIDReplay       = "replay"
	dialogIDPalette      = "palette"
	dialogIDConfig       = "config"
)

const (
	planApprovalPurpose     = "plan_approval"
	planApprovalSaveLabel   = "Save plan and proceed"
	planApprovalGoLabel     = "Proceed without saving plan files"
	planApprovalRejectLabel = "Reject plan"
)

// App is the root Bubble Tea model that routes between home and session views.
type App struct {
	keys    KeyMap
	home    Home
	session Session
	dialogs []tea.Model

	route route

	// sessionID is the active session's server-assigned ID.
	sessionID string

	// api is the application boundary for the TUI. It can be backed by the
	// embedded HTTP server or by an in-process adapter.
	api Backend

	// ctx is the app-level context, cancelled on quit. All tea.Cmd closures
	// should capture this instead of using context.Background().
	ctx    context.Context    //nolint:containedctx // intentional: Bubbletea Cmd closures need a cancellable context
	cancel context.CancelFunc //nolint:containedctx

	// sse manages the SSE connection lifecycle for the active session.
	sse sseManager

	width  int
	height int
	ready  bool

	theme     *theme.Theme
	themeName string // bare name of the active theme ("" = system palette)

	// Cached status bar data from /config. It survives session resets.
	sbToolCount  int
	sbMCPServers []MCPServerStatus
	sbGitBranch  string

	pendingAttachments []Attachment
	pendingUndoFile    string
	maxAttachmentSize  int64
	displayTurns       int
	cfg                ConfigState // model, agent, variant selection
	agentPersistDirty  bool
	agentPersistSeq    int
	pins               []string
	memoryStore        *service.MemoryStore
	errorBanner        string // global error banner, shown at top of screen
	infoBanner         string // global info banner (success/info), shown at top of screen
	cancelRequested    bool   // true when the user explicitly requested turn cancellation
	quitting           bool
	autoResume         bool

	lspManager *lsp.Manager
	projectDir string
	taskStore  *tool.TaskStore

	stepTraces   [][]stepTraceTUI
	traceEnabled bool
}

func NewApp(apiBase string, th *theme.Theme) App {
	return NewAppWithBackend(NewAPIClient(apiBase), th)
}

func NewAppWithBackend(backend Backend, th *theme.Theme) App {
	wd, _ := os.Getwd()
	ctx, cancel := context.WithCancel(context.Background())
	app := App{
		keys:       DefaultKeyMap(),
		home:       NewHome(),
		session:    NewSession(),
		route:      routeHome,
		api:        backend,
		theme:      th,
		cfg:        NewConfigState(),
		projectDir: wd,
		ctx:        ctx,
		cancel:     cancel,
		taskStore:  tool.NewTaskStore(nil),
	}

	if th != nil {
		app.home.ApplyTheme(th)
		app.session.ApplyTheme(th)
	}
	return app
}

type RunOpts struct {
	Resume      bool
	LSPManager  *lsp.Manager
	MemoryStore *service.MemoryStore
	TaskStore   *tool.TaskStore
}

func Run(apiBase string, opts ...RunOpts) error {
	client := NewAPIClient(apiBase)
	if cfg, err := config.Load(""); err == nil && cfg.TUI.SSEReadTimeout > 0 {
		client.SSEReadTimeout = time.Duration(cfg.TUI.SSEReadTimeout) * time.Minute
	}
	return RunWithBackend(client, opts...)
}

func RunWithBackend(backend Backend, opts ...RunOpts) error {
	var opt RunOpts
	if len(opts) > 0 {
		opt = opts[0]
	}

	cfg, _ := config.Load("")
	var th *theme.Theme
	var themePath string

	themeName := ""
	if cfg != nil {
		themeName = cfg.TUI.Theme
	}

	if dbTheme, err := backend.GetSetting(context.Background(), "tui.theme"); err == nil {
		themeName = dbTheme
	}

	if themeName != "" {
		themePath = filepath.Join(config.ThemesDir(), themeName+".yaml")
		loaded, err := theme.NewLoader(theme.LoaderConfig{Path: themePath}).Load()
		if err != nil {
			log.Printf("theme: failed to load %s: %v — using default", themePath, err)
			d := theme.StaticDefault()
			th = &d
			themePath = ""
		} else {
			th = loaded
		}
	} else {
		p, err := termpalette.Detect(os.Stdin, os.Stdout, 200*time.Millisecond)
		if err != nil {
			d := theme.StaticDefault()
			th = &d
		} else {
			derived := theme.FromPalette(p)
			th = &derived
		}
	}

	app := NewAppWithBackend(backend, th)
	app.themeName = themeName
	app.autoResume = opt.Resume
	app.lspManager = opt.LSPManager
	app.memoryStore = opt.MemoryStore
	if opt.TaskStore != nil {
		app.taskStore = opt.TaskStore
	}
	if cfg != nil {
		app.displayTurns = cfg.TUI.DisplayTurns
		app.maxAttachmentSize = cfg.TUI.MaxAttachmentSize
	}

	prog := tea.NewProgram(app)

	if themePath != "" {
		w, err := theme.NewWatcher(themePath, func(t *theme.Theme) {
			prog.Send(ThemeChangedMsg{Theme: t})
		})
		if err == nil {
			defer w.Close()
		}
	}

	_, err := prog.Run()
	return err
}

// Init satisfies tea.Model. It delegates to the home screen so that the
// textarea focus command is dispatched on startup, and fetches the server
// config so the default model can be shown on the home screen.
func (a App) Init() tea.Cmd {
	return a.fetchInitialConfig()
}
