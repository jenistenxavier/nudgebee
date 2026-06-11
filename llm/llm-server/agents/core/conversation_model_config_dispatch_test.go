package core

import (
	"testing"

	"nudgebee/llm/security"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// modelConfigDaoStub satisfies IConversationDao by embedding the interface;
// only the two updaters we exercise are implemented. Any other method would
// panic with a nil pointer — which is fine because applyConversationModelConfig
// only calls these two.
type modelConfigDaoStub struct {
	IConversationDao
	blanketCalls []blanketCall
	tierCalls    []tierCall
	blanketErr   error
	tierErr      error
}

type blanketCall struct {
	conversationID, provider, model string
}
type tierCall struct {
	conversationID string
	overrides      ConversationTierOverrides
}

func (s *modelConfigDaoStub) UpdateConversationModelBlanket(conversationId, provider, model string) error {
	s.blanketCalls = append(s.blanketCalls, blanketCall{conversationId, provider, model})
	return s.blanketErr
}
func (s *modelConfigDaoStub) UpdateConversationTierOverrides(conversationId string, overrides ConversationTierOverrides) error {
	s.tierCalls = append(s.tierCalls, tierCall{conversationId, overrides})
	return s.tierErr
}

func TestApplyConversationModelConfig_TierWinsWhenBothSet(t *testing.T) {
	ctx := security.NewRequestContextForSuperAdmin()
	stub := &modelConfigDaoStub{}
	tierOverrides := ConversationTierOverrides{Picks: map[string]TierModelPick{
		"reasoning": {Provider: "googleai", Model: "gemini-2.5-pro"},
	}}

	applyConversationModelConfig(ctx, stub, "conv-1", "openai", "gpt-4", tierOverrides)

	require.Len(t, stub.tierCalls, 1, "tier dispatch must run")
	assert.Equal(t, "conv-1", stub.tierCalls[0].conversationID)
	assert.Equal(t, tierOverrides, stub.tierCalls[0].overrides)
	assert.Empty(t, stub.blanketCalls, "blanket dispatch must NOT run when tier is set")
}

func TestApplyConversationModelConfig_TierOnly(t *testing.T) {
	ctx := security.NewRequestContextForSuperAdmin()
	stub := &modelConfigDaoStub{}
	tierOverrides := ConversationTierOverrides{Picks: map[string]TierModelPick{
		"retrieval": {Provider: "openai", Model: "gpt-4o-mini"},
	}}

	applyConversationModelConfig(ctx, stub, "conv-2", "", "", tierOverrides)

	require.Len(t, stub.tierCalls, 1)
	assert.Empty(t, stub.blanketCalls)
}

func TestApplyConversationModelConfig_BlanketOnly(t *testing.T) {
	ctx := security.NewRequestContextForSuperAdmin()
	stub := &modelConfigDaoStub{}

	applyConversationModelConfig(ctx, stub, "conv-3", "anthropic", "claude-opus-4-7", ConversationTierOverrides{})

	require.Len(t, stub.blanketCalls, 1, "blanket dispatch must run")
	assert.Equal(t, "conv-3", stub.blanketCalls[0].conversationID)
	assert.Equal(t, "anthropic", stub.blanketCalls[0].provider)
	assert.Equal(t, "claude-opus-4-7", stub.blanketCalls[0].model)
	assert.Empty(t, stub.tierCalls, "tier dispatch must NOT run when only blanket is set")
}

func TestApplyConversationModelConfig_NoopWhenBothEmpty(t *testing.T) {
	ctx := security.NewRequestContextForSuperAdmin()
	stub := &modelConfigDaoStub{}

	applyConversationModelConfig(ctx, stub, "conv-4", "", "", ConversationTierOverrides{})

	assert.Empty(t, stub.blanketCalls)
	assert.Empty(t, stub.tierCalls)
}

func TestApplyConversationModelConfig_PartialBlanketIgnored(t *testing.T) {
	ctx := security.NewRequestContextForSuperAdmin()
	stub := &modelConfigDaoStub{}

	// Provider without model — too incomplete to act on (caller's responsibility).
	applyConversationModelConfig(ctx, stub, "conv-5", "openai", "", ConversationTierOverrides{})

	assert.Empty(t, stub.blanketCalls, "must not dispatch with half-set blanket pair")
	assert.Empty(t, stub.tierCalls)
}
