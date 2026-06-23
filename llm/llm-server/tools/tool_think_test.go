package tools

import (
	"testing"

	core "nudgebee/llm/tools/core"

	"github.com/stretchr/testify/assert"
)

func TestThinkTool_EchoesReasoning(t *testing.T) {
	tool := &thinkTool{}
	resp, err := tool.Call(core.NbToolContext{}, core.NBToolCallRequest{
		Arguments: map[string]any{
			"reasoning": "Logs show connection refused but metrics show low CPU. This suggests the issue is not resource exhaustion but rather a network partition or misconfigured service endpoint.",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, core.NBToolResponseStatusSuccess, resp.Status)
	assert.Contains(t, resp.Data, "network partition")
}

func TestThinkTool_FallbackToCommand(t *testing.T) {
	tool := &thinkTool{}
	resp, err := tool.Call(core.NbToolContext{}, core.NBToolCallRequest{
		Command: "Need to reconsider the approach",
	})
	assert.NoError(t, err)
	assert.Equal(t, core.NBToolResponseStatusSuccess, resp.Status)
	assert.Contains(t, resp.Data, "reconsider")
}

func TestThinkTool_EmptyArgsFallsBackToCommand(t *testing.T) {
	tool := &thinkTool{}
	resp, err := tool.Call(core.NbToolContext{}, core.NBToolCallRequest{
		Command:   "fallback reasoning",
		Arguments: map[string]any{},
	})
	assert.NoError(t, err)
	assert.Equal(t, "fallback reasoning", resp.Data)
}

func TestThinkTool_Metadata(t *testing.T) {
	tool := &thinkTool{}
	assert.Equal(t, "think", tool.Name())
	assert.Equal(t, core.NBToolTypeTool, tool.GetType())
	assert.Contains(t, tool.Description(), "conflicting evidence")

	schema := tool.InputSchema()
	assert.Equal(t, core.ToolSchemaTypeObject, schema.Type)
	assert.Contains(t, schema.Properties, "reasoning")
	assert.Equal(t, []string{"reasoning"}, schema.Required)
}

// TestThinkTool_DescriptionForbidsNarration pins the narration-misuse
// guardrails introduced after 2026-06-22 baseline (70% of calls were
// "ready to provide final answer" narration despite the May rewrite).
// The description must name the forbidden phrasings explicitly so the
// LLM has something concrete to match against, and must call out the
// notebook/hypothesis-tree overlap to suppress think-as-bookkeeping use.
func TestThinkTool_DescriptionForbidsNarration(t *testing.T) {
	tool := &thinkTool{}
	desc := tool.Description()

	// Specific narration phrasings sampled from production misuse.
	// Pin ALL forbidden phrasings listed in the description so a future
	// rewrite cannot silently drop one (per Gemini PR #32697 review).
	for _, banned := range []string{
		"ready to provide (the) final answer",
		"ready to give (the) answer",
		"will provide (the) final answer",
		"investigation is complete",
		"notebook updated",
		"i have enough information to answer",
		"consolidated findings: ... ready to ...",
	} {
		assert.Contains(t, desc, banned,
			"description must explicitly name the %q narration pattern so the LLM can match against it", banned)
	}

	// Hypothesis-mode redundancy must be called out so the agent stops
	// using `think` as a notebook-update proxy when the hypothesis tree
	// already provides structured reasoning.
	assert.Contains(t, desc, "hypothesis-mode",
		"description must call out think/notebook overlap (think is redundant on top of the hypothesis tree)")
	assert.Contains(t, desc, "[SUPPORTED]",
		"description must point at the notebook's hypothesis-status markers as the canonical reasoning surface")

	// Positive guidance must still be present.
	assert.Contains(t, desc, "names a SPECIFIC choice",
		"description must define what a valid think call looks like, not just forbid bad ones")
}
