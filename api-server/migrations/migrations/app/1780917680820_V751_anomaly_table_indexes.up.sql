-- Add indexes that cover the hot read paths on `anomaly`:
--   1. (account_id, anomaly_type)
--        - api-server/services/anomoly/service.go fetchExistingAnomalyCounts
--          (batched COUNT(*) ... WHERE account_id IN (?) GROUP BY account_id, anomaly_type)
--        - spend_anomaly.go getOpenSpendAnomalies / hasOpenAnomaly (combined with the
--          partial index below)
--
--   2. (account_id, evaluated_at DESC)
--        - query/metadata.go anomaly_v3 listing view
--          (WHERE evaluated_at >= NOW() - INTERVAL '1 month' with row-level
--          account_id = $ injection)
--        - LLM agent SQL tool queries that ORDER BY evaluated_at DESC LIMIT N
--          for a given account scope
--
--   3. (account_id, anomaly_type, name) WHERE anomaly_status = 'OPEN'
--        - Replaces idx_anomaly_status_type (which lacked account_id and so still
--          required a tenant-wide partial-index scan + filter).
--        - Covers getOpenSpendAnomalies (account_id + anomaly_type IN) and
--          hasOpenAnomaly (account_id + anomaly_type + name).
--
-- These run as plain CREATE INDEX (inside golang-migrate's default transaction),
-- which takes a brief ACCESS EXCLUSIVE lock on `anomaly`. If the table grows large
-- enough that this lock becomes problematic, future migrations can move to
-- CONCURRENTLY + `-- migrate:no-transaction`.

CREATE INDEX IF NOT EXISTS idx_anomaly_account_type
    ON public.anomaly (account_id, anomaly_type);

CREATE INDEX IF NOT EXISTS idx_anomaly_account_evaluated_at
    ON public.anomaly (account_id, evaluated_at DESC);

CREATE INDEX IF NOT EXISTS idx_anomaly_open_account_type_name
    ON public.anomaly (account_id, anomaly_type, name)
    WHERE anomaly_status = 'OPEN';

DROP INDEX IF EXISTS public.idx_anomaly_status_type;
