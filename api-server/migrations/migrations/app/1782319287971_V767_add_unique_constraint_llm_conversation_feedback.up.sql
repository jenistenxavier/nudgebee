
ALTER TABLE "public"."llm_conversation_feedback"
  ADD CONSTRAINT "llm_conversation_feedback_session_conversation_user_account_key"
  UNIQUE (session_id, conversation_id, user_id, cloud_account_id);
