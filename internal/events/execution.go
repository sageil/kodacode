package events

type ExecutionStatus string

const (
	ExecutionStatusPendingApproval ExecutionStatus = "pendingApproval"
	ExecutionStatusInProgress      ExecutionStatus = "inProgress"
	ExecutionStatusCompleted       ExecutionStatus = "completed"
	ExecutionStatusFailed          ExecutionStatus = "failed"
	ExecutionStatusDeclined        ExecutionStatus = "declined"
)

type ExecutionState struct {
	ExecutionID      string
	ToolCallID       string
	ToolName         string
	Kind             string
	Intent           string
	Effect           string
	Command          []string
	CommandPreview   string
	WorkingDirectory string
	TimeoutMS        int64
	OutputLimit      int
	Status           ExecutionStatus
	Input            string
	Output           string
	Error            string
	OutputBlob       *ToolResultBlobRef
	ErrorBlob        *ToolResultBlobRef
	OutputTruncated  bool
	ErrorTruncated   bool
	ExitCode         *int
	DurationMS       int64
	CommandActions   []string
	Executing        bool
	Completed        bool
	Runtime          *ToolExecRuntimeState
	Background       *ExecutionBackgroundState
}

type ExecutionBackgroundStatus string

const (
	ExecutionBackgroundStatusStarting        ExecutionBackgroundStatus = "starting"
	ExecutionBackgroundStatusRunning         ExecutionBackgroundStatus = "running"
	ExecutionBackgroundStatusReady           ExecutionBackgroundStatus = "ready"
	ExecutionBackgroundStatusExited          ExecutionBackgroundStatus = "exited"
	ExecutionBackgroundStatusSupervisionLost ExecutionBackgroundStatus = "supervisionLost"
)

type ExecutionBackgroundState struct {
	PID             int
	ProcessIdentity string
	SupervisorID    string
	Status          ExecutionBackgroundStatus
	LogRef          string
	ReadyPatterns   []string
	Started         bool
	StartedAtSeq    int64
	Ready           bool
	ReadyAtSeq      int64
	ReadyMessage    string
	Port            int
	OutputTail      string
	OutputBytes     int64
	Exited          bool
	ExitedAtSeq     int64
	ExitCode        *int
	Error           string
}

type ExecutionApprovalState struct {
	RequestID             string
	ExecutionID           string
	TurnID                string
	ToolCallID            string
	ToolName              string
	Command               string
	WorkingDirectory      string
	Reason                string
	PrefixRule            []string
	SessionGrantPaths     []string
	NetworkTargets        []string
	AvailableDecisions    []ExecutionApprovalDecision
	ProposedExecPolicy    *ExecutionPolicyAmendment
	ProposedNetworkPolicy *ExecutionNetworkPolicyAmendment
	RequestedAtSeq        int64
}

type ApprovedExecutionState struct {
	RequestID            string
	ExecutionID          string
	TurnID               string
	ToolCallID           string
	ToolName             string
	Command              string
	WorkingDirectory     string
	Decision             ExecutionApprovalDecision
	AppliedExecPolicy    *ExecutionPolicyAmendment
	AppliedNetworkPolicy *ExecutionNetworkPolicyAmendment
	ApprovedAtSeq        int64
}
