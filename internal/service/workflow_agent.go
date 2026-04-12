package service

const (
	engineerAgentID = "engineer"
)

func isEngineerWorkflowAgent(agentID string) bool {
	return agentID == engineerAgentID
}
