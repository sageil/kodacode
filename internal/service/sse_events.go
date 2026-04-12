package service

import "github.com/sageil/kodacode/v1/internal/provider"

type SSEEvent struct {
	Type string
	Data any
}

type SSEDeltaData struct {
	Content string `json:"content"`
}

type SSEAssistantMessageData struct {
	Content string `json:"content"`
}

type SSESystemMessageData struct {
	Content string `json:"content"`
}

type SSEToolStartData struct {
	Tool   string `json:"tool"`
	Input  string `json:"input"`
	CallID string `json:"call_id,omitempty"`
}

type SSEToolInputDeltaData struct {
	Tool   string `json:"tool"`
	Delta  string `json:"delta"`
	CallID string `json:"call_id,omitempty"`
}

type SSEToolEndData struct {
	Tool   string  `json:"tool"`
	Output string  `json:"output"`
	Error  *string `json:"error"`
	CallID string  `json:"call_id,omitempty"`
}

type SSEDoneData struct {
	Usage           *provider.Usage `json:"usage"`
	ContextSize     int             `json:"context_size"`
	MaxInputTokens  int             `json:"max_input_tokens,omitempty"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	SessionCost     float64         `json:"session_cost"`
	SubagentCost    float64         `json:"subagent_cost,omitempty"`
	BudgetWarn      bool            `json:"budget_warn,omitempty"`
	CostSnapshot    *CostSnapshot   `json:"cost_snapshot,omitempty"`
}

type SSEErrorData struct {
	Message string `json:"message"`
}

type SSEQuestionData struct {
	QuestionID       string `json:"question_id"`
	Tool             string `json:"tool"`
	Input            string `json:"input"`
	SuggestedPattern string `json:"suggested_pattern"`
}

type SSEReasoningDeltaData struct {
	Content string `json:"content"`
}

type SSEReasoningDoneData struct {
	Tokens int `json:"tokens"`
}

type SSETitleData struct {
	Title string `json:"title"`
}

type SSECompactionData struct {
	Summary string `json:"summary"`
}

type SSEUserQuestionData struct {
	QuestionID string   `json:"question_id"`
	Question   string   `json:"question"`
	Options    []string `json:"options"`
	Multiple   bool     `json:"multiple"`
	Purpose    string   `json:"purpose,omitempty"`
}

type SSEBackgroundEventData struct {
	TaskID         string `json:"task_id"`
	Command        string `json:"command"`
	Description    string `json:"description,omitempty"`
	ExitCode       int    `json:"exit_code"`
	Output         string `json:"output"`
	ElapsedMS      int64  `json:"elapsed_ms"`
	Status         string `json:"status"`
	AutoReactState string `json:"auto_react_state,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

type SSESubagentActivityData struct {
	Tool   string `json:"tool"`
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
	Done   bool   `json:"done"`
	Error  bool   `json:"error,omitempty"`
}

type SSETaskSyncData struct {
	ActiveTaskID string `json:"active_task_id,omitempty"`
}

type SSETraceData struct {
	TurnIndex int         `json:"turn_index"`
	Steps     []StepTrace `json:"steps"`
}

type SSEOverflowData struct {
	Dropped  int  `json:"dropped"`
	Critical bool `json:"critical,omitempty"`
}
