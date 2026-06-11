-- Adds llm_tier_overrides jsonb column to llm_conversations to store the
-- per-category (tier) model selection that a user picks on the chat UI's
-- "Per category" mode. The blanket-mode selection still uses the existing
-- llm_provider + llm_model columns (added in V629); the DAO enforces mutual
-- exclusivity — writing one mode clears the other in the same UPDATE so a
-- reader never sees both set.
--
-- Shape stored when populated:
--
--   {
--     "picks": {
--       "reasoning": { "provider": "anthropic", "model": "claude-opus-4-7" },
--       "retrieval": { "provider": "googleai",  "model": "gemini-2.5-flash" },
--       "summary":   { "provider": "anthropic", "model": "claude-haiku-3.5" }
--     }
--   }
--
-- A nullable jsonb scales to future categories without further migrations —
-- adding a new ModelTier on the backend just means writing a new key.
ALTER TABLE public.llm_conversations
  ADD COLUMN IF NOT EXISTS llm_tier_overrides jsonb NULL;

COMMENT ON COLUMN public.llm_conversations.llm_tier_overrides IS
  'Per-category (tier) model picks for the conversation. JSON shape: {"picks":{"<tier>":{"provider":"...","model":"..."},...}}. NULL when blanket mode (llm_provider+llm_model) is in use. Mutually exclusive with llm_provider+llm_model at the DAO level.';
