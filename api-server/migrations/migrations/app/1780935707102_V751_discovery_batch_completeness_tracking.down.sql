DROP TABLE IF EXISTS "public"."discovery_batch_progress";

DROP INDEX IF EXISTS "public"."idx_active_resources_account_tenant_type_batch";

ALTER TABLE "public"."active_resources" DROP COLUMN IF EXISTS "batch_id";
