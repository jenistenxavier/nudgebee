-- Rollback V751. Idempotent.
ALTER TABLE public.llm_conversations
  DROP COLUMN IF EXISTS llm_tier_overrides;
