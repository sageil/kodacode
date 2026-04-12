package pipeline

// WorkflowPhase is the current engineer workflow phase for the turn.
type WorkflowPhase string

const (
	WorkflowPhaseUnknown          WorkflowPhase = ""
	WorkflowPhasePrebuild         WorkflowPhase = "prebuild"
	WorkflowPhasePreplan          WorkflowPhase = "preplan"
	WorkflowPhasePostplanPending  WorkflowPhase = "postplan-pending"
	WorkflowPhasePostplanRejected WorkflowPhase = "postplan-rejected"
	WorkflowPhaseApproved         WorkflowPhase = "approved"
)

// WorkflowApprovalStatus is the structured status of the latest/effective
// plan approval state.
type WorkflowApprovalStatus string

const (
	WorkflowApprovalNone     WorkflowApprovalStatus = ""
	WorkflowApprovalPending  WorkflowApprovalStatus = "pending"
	WorkflowApprovalApproved WorkflowApprovalStatus = "approved"
	WorkflowApprovalRejected WorkflowApprovalStatus = "rejected"
)

// WorkflowPlanState stores structured planner approval state for the turn.
type WorkflowPlanState struct {
	LatestStatus          WorkflowApprovalStatus
	EffectiveStatus       WorkflowApprovalStatus
	LatestAnswer          string
	PendingQuestionID     string
	PriorApprovedInEffect bool
}

// WorkflowState is hydrated once from message history and then updated
// incrementally by the turn loop as workflow events occur.
type WorkflowState struct {
	Phase            WorkflowPhase
	HasCalledTest    bool
	HasCalledPlanner bool
	Plan             WorkflowPlanState
}
