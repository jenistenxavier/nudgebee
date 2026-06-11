-- Partial index to fix the discovery resource-deletion cleanup in
-- collector-server/k8s-collector/app/handlers/discovery_handler.py
-- (handle_active_resources_deletion / handle_full_load_deleted_resources).
--
-- Those UPDATEs flip stale rows to is_active=false with the predicate:
--   WHERE account = %s AND type = '...' AND is_active IS NOT FALSE
-- NULL is treated as "active" system-wide (the `is_active IS NOT FALSE`
-- convention used by the K8s aggregation materialized views), so the cleanup
-- must keep matching NULL rows — it cannot narrow to `is_active = true`.
--
-- The pre-existing partial index idx_cloud_resourses_active_account_type is
-- defined `WHERE is_active = true`, which the planner CANNOT use for an
-- `IS NOT FALSE` predicate (the predicate is broader — it includes NULL). With
-- no matching index, each cleanup bitmap-heap-scans the account's ENTIRE
-- resource history for the type (e.g. ~76k Pod rows for one busy account, 99%
-- already inactive) and grows unboundedly as churned pods accumulate. Observed
-- degrading from ~55s to ~460s per call, exceeding the 300s discovery lock
-- wait and causing "Could not acquire discovery lock" rejections.
--
-- This index restricts to the currently-active set (is_active IS NOT FALSE)
-- keyed by (account, type), so the cleanup touches only the live rows
-- (hundreds) instead of the full history.
--
-- Plain CREATE INDEX (no CONCURRENTLY): golang-migrate wraps migrations in a
-- transaction, which CONCURRENTLY cannot run inside. This takes an ACCESS
-- EXCLUSIVE lock on cloud_resourses for the build. It is a PARTIAL index
-- (is_active IS NOT FALSE ~= 1-2M of ~15M rows), so the lock is held only for
-- the scan/build (a couple of minutes), once. On a cluster where that stall is
-- unacceptable, an operator may pre-build it out-of-band:
--   CREATE INDEX CONCURRENTLY idx_cloud_resourses_active_notfalse_account_type
--       ON public.cloud_resourses (account, type) WHERE is_active IS NOT FALSE;
-- after which this migration is a no-op via IF NOT EXISTS.

CREATE INDEX IF NOT EXISTS idx_cloud_resourses_active_notfalse_account_type
    ON public.cloud_resourses (account, type)
    WHERE is_active IS NOT FALSE;
