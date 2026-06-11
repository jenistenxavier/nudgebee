-- Snapshot batching contract: make the discovery deletion-reconcile safe under
-- unordered, concurrent, multi-pod batch processing.
--
-- The k8s-agent's full-load snapshot can now arrive as N batches sharing one
-- batch_id. The collector consumes the discovery queue with multiple workers
-- across multiple pods, so a single snapshot's batches are processed out of
-- order. Reconcile (mass-deactivate resources absent from the snapshot) must
-- therefore fire only once the whole sequence 1..N for a batch_id has landed,
-- not on a bare is_last_batch. These two changes provide the shared, cross-pod
-- state that makes that possible.

-- 1. Tag each staged active_resources row with the snapshot batch_id that wrote
--    it, so reconcile can diff the live set against ONLY the completed snapshot.
--    NULL = legacy / incremental path (single atomic message, batch_id absent).
ALTER TABLE "public"."active_resources" ADD COLUMN IF NOT EXISTS "batch_id" text;

-- Reconcile and GC both filter by (account, tenant, resource_type, batch_id).
CREATE INDEX IF NOT EXISTS "idx_active_resources_account_tenant_type_batch" ON
  "public"."active_resources" USING btree ("cloud_account_id", "tenant_id", "resource_type", "batch_id");

-- 2. Per-batch arrival ledger. One row per (snapshot, sequence). The completing
--    worker detects "sequence 1..N all present" from this table and atomically
--    claims the reconcile by DELETE-ing the rows (rowcount>0 == claimed), so
--    exactly one pod reconciles even with the cross-pod Redis lock degraded.
CREATE TABLE IF NOT EXISTS "public"."discovery_batch_progress" (
  "cloud_account_id" uuid NOT NULL,
  "tenant_id" uuid NOT NULL,
  "data_type" text NOT NULL,
  "batch_id" text NOT NULL,
  "batch_sequence" integer NOT NULL,
  "is_last" boolean NOT NULL DEFAULT false,
  "seen_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("cloud_account_id", "tenant_id", "data_type", "batch_id", "batch_sequence")
);

-- Age-based sweep for snapshots that never complete (lost/DLQ'd middle batch,
-- or an account that disappears mid-snapshot).
CREATE INDEX IF NOT EXISTS "idx_discovery_batch_progress_seen_at" ON
  "public"."discovery_batch_progress" USING btree ("seen_at");
