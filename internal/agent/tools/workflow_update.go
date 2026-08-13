package tools

import (
	"context"
	"fmt"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/workflow"
)

const WorkflowUpdateToolName = "workflow_update"

type WorkflowUpdateParams struct {
	Data map[string]any `json:"data" description:"Durable workflow memory. Most fields merge into the current worker. Use top-level findings as an object keyed by stable finding ID; each finding may use status open, resolved, or superseded. Store only concise facts and file/symbol/range references; never full source code or raw tool output."`
}

// NewWorkflowUpdateTool creates a workflow-memory tool scoped by the current
// Crush session. The model cannot select a workflow file or worker name.
func NewWorkflowUpdateTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		WorkflowUpdateToolName,
		"Persist concise durable memory for the active workflow. Most data is scoped to the current worker; top-level findings are shared across workers and are keyed by stable IDs such as review-001. New findings default to open and record the current worker as source; updates merge into an existing ID. Reconcile findings as resolved or superseded when work makes that clear. Do not store full source code or raw tool output.",
		func(ctx context.Context, params WorkflowUpdateParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for workflow updates")
			}
			worker, source, err := workflow.UpdateWorkflowMemory(workingDir, sessionID, params.Data)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			return fantasy.NewTextResponse(fmt.Sprintf("Workflow memory updated for %s in %s.", worker, source)), nil
		},
	)
}
