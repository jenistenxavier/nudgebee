package tools

import (
	core "nudgebee/llm/tools/core"
)

// ThinkToolName is the registered name for the think tool.
const ThinkToolName = "think"

func init() {
	core.RegisterNBToolFactory(ThinkToolName, func(accountId string) (core.NBTool, error) {
		return &thinkTool{}, nil
	})
}

type thinkTool struct{}

func (t *thinkTool) Name() string             { return ThinkToolName }
func (t *thinkTool) GetType() core.NBToolType { return core.NBToolTypeTool }

func (t *thinkTool) Description() string {
	return "Record reasoning that changes your next action. " +
		"USE for: conflicting evidence to reconcile; stuck after 3+ tool calls; multiple root causes to weigh; tool error/empty result (decide retry, pivot, or honest failure). " +
		"A valid think call names a SPECIFIC choice (this-vs-that, retry-vs-pivot, fetch-X-or-Y-first) AND the reason behind it. If there is no choice being weighed, do not call this tool. " +
		"DO NOT use as a transition signal or status announcement — emit '<final_answer>' directly when you are ready, do not call 'think' to announce it. " +
		"Forbidden phrasings include: 'ready to provide (the) final answer', 'ready to give (the) answer', 'will provide (the) final answer', 'investigation is complete', 'notebook updated', 'i have enough information to answer', 'consolidated findings: ... ready to ...'. These narrate an upcoming action; they are not reasoning that changes it. " +
		"DO NOT restate prior findings. DO NOT replace a tool call. DO NOT use 'think' to record a notebook update — if you are in hypothesis-mode the notebook ('## Hypothesis Tree', '[SUPPORTED]'/'[REFUTED]' markers) is the structured place for that, and 'think' is redundant on top of it. " +
		"Max 2 per investigation."
}

func (t *thinkTool) InputSchema() core.ToolSchema {
	return core.ToolSchema{
		Type: core.ToolSchemaTypeObject,
		Properties: map[string]core.ToolSchemaProperty{
			"reasoning": {
				Type:        core.ToolSchemaTypeString,
				Description: "The conflict, stuck point, candidate root causes, or tool error you face — and the next action it leads to. Not for restating findings or announcing the final answer.",
			},
		},
		Required: []string{"reasoning"},
	}
}

func (t *thinkTool) Call(_ core.NbToolContext, input core.NBToolCallRequest) (core.NBToolResponse, error) {
	reasoning := input.Command
	if input.Arguments != nil {
		if r, ok := input.Arguments["reasoning"].(string); ok && r != "" {
			reasoning = r
		}
	}
	return core.NBToolResponse{
		Data:   reasoning,
		Status: core.NBToolResponseStatusSuccess,
	}, nil
}
