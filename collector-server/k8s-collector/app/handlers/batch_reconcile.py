"""Snapshot-batch completeness gating for discovery reconcile.

A full-load discovery snapshot can arrive as N batches sharing one ``batch_id``.
The discovery queue is consumed by multiple workers across multiple collector
pods, so a snapshot's batches are processed concurrently and OUT OF ORDER. The
deletion-reconcile (mark resources absent from the snapshot inactive) must
therefore fire only once the whole sequence ``1..N`` for a ``batch_id`` has
landed -- never on a bare ``is_last_batch``, which may be processed before the
batches that carry the bulk of the resources.

Design:

* Every batch records its arrival in ``discovery_batch_progress`` AFTER it has
  stored its resources into ``active_resources`` (tagged with ``batch_id``). So
  "progress row for seq k exists" implies "seq k's active_resources committed".
* Completeness is derived from the shared table, not from ``total_batches``
  (which the agent over-estimates -- converters drop items): the ``is_last``
  batch carries the true final sequence N, and the snapshot is complete when all
  of ``1..N`` are present. The agent emits contiguous sequences, so
  ``count(distinct sequence) == max(is_last sequence)`` is exact.
* The completing worker CLAIMS the reconcile with a single atomic
  ``DELETE ... RETURNING`` over the progress rows. Postgres row-locks serialize
  concurrent claimers, so exactly one pod reconciles even when the cross-pod
  Redis lock is degraded (Redis down) -- correctness does not depend on it.

The legacy / incremental path (no ``batch_id``) is untouched: a single atomic
full-load message clears + stores + reconciles in one shot, and incremental
updates never reconcile.
"""

import logging

from db import database

# Orphaned snapshots (a lost/DLQ'd middle batch, or an account that disappears
# mid-snapshot) leave progress rows that never complete. Sweep them when a
# reconcile claim succeeds so the table stays bounded without a dedicated cron.
_STALE_PROGRESS_MAX_AGE_SECONDS = 2 * 60 * 60  # 2h -- well past any resync


def record_batch_progress(cloud_account_id, tenant, data_type, batch_id, batch_sequence, is_last):
    """Idempotently record that (batch_id, batch_sequence) has arrived.

    Idempotent on redelivery: ON CONFLICT keeps the row and refreshes is_last /
    seen_at so a redelivered last-batch still marks the terminal sequence.
    """
    try:
        database.run_query(
            "INSERT INTO discovery_batch_progress "
            "(cloud_account_id, tenant_id, data_type, batch_id, batch_sequence, is_last) "
            "VALUES (%s, %s, %s, %s, %s, %s) "
            "ON CONFLICT (cloud_account_id, tenant_id, data_type, batch_id, batch_sequence) "
            "DO UPDATE SET is_last = discovery_batch_progress.is_last OR EXCLUDED.is_last, seen_at = now()",
            [cloud_account_id, tenant, data_type, batch_id, batch_sequence, bool(is_last)],
        )
    except Exception:
        logging.exception(
            f"Failed to record batch progress for {cloud_account_id}/{tenant}/{data_type}/{batch_id}"
            f" seq {batch_sequence}"
        )


def claim_reconcile(cloud_account_id, tenant, data_type, batch_id) -> bool:
    """Atomically claim the reconcile for a snapshot iff its sequence is complete.

    Returns True for exactly one caller across all pods/workers once every
    sequence ``1..N`` for the batch_id is present (N = the is_last sequence).
    The claim DELETEs the progress rows in a single statement; Postgres row
    locks ensure a concurrent claimer sees them gone and returns False.

    Completeness: ``count(distinct sequence) == max(sequence where is_last)``.
    Until the is_last batch arrives, max(...) is NULL so the equality is NULL
    (false) and nothing is claimed. A missing middle sequence makes the count
    fall short, so the snapshot stays fail-closed (no reconcile, no mass
    deactivation) until it completes or is swept.
    """
    try:
        claimed = database.run_query(
            "DELETE FROM discovery_batch_progress p "
            "WHERE p.cloud_account_id = %s AND p.tenant_id = %s AND p.data_type = %s AND p.batch_id = %s "
            "AND ( "
            "  SELECT count(DISTINCT c.batch_sequence) FROM discovery_batch_progress c "
            "  WHERE c.cloud_account_id = p.cloud_account_id AND c.tenant_id = p.tenant_id "
            "    AND c.data_type = p.data_type AND c.batch_id = p.batch_id "
            ") = ( "
            "  SELECT max(m.batch_sequence) FROM discovery_batch_progress m "
            "  WHERE m.cloud_account_id = p.cloud_account_id AND m.tenant_id = p.tenant_id "
            "    AND m.data_type = p.data_type AND m.batch_id = p.batch_id AND m.is_last = true "
            ") "
            "RETURNING p.batch_sequence",
            [cloud_account_id, tenant, data_type, batch_id],
        )
        if claimed:
            _sweep_stale_progress(cloud_account_id)
            return True
        return False
    except Exception:
        logging.exception(
            f"Failed to claim reconcile for {cloud_account_id}/{tenant}/{data_type}/{batch_id}; "
            f"skipping cleanup (fail-closed)"
        )
        return False


def gc_superseded_active_resources(cloud_account_id, tenant, resource_type, current_batch_id):
    """Drop active_resources rows tagged with a superseded batch_id.

    Run after a successful reconcile. Removes the previous snapshot's rows and
    any rows left by orphaned (never-completed) snapshots of the same type, so
    the table stays bounded. NULL-tagged (legacy) rows are left alone.
    """
    try:
        database.run_query(
            "DELETE FROM active_resources "
            "WHERE cloud_account_id = %s AND tenant_id = %s AND resource_type = %s "
            "AND batch_id IS NOT NULL AND batch_id <> %s",
            [cloud_account_id, tenant, resource_type, current_batch_id],
        )
    except Exception:
        logging.exception(f"Failed to GC superseded active_resources for {cloud_account_id}/{tenant}/{resource_type}")


def _sweep_stale_progress(cloud_account_id):
    """Delete progress rows for snapshots that never completed (age-based)."""
    try:
        database.run_query(
            "DELETE FROM discovery_batch_progress "
            "WHERE cloud_account_id = %s AND seen_at < now() - make_interval(secs => %s)",
            [cloud_account_id, _STALE_PROGRESS_MAX_AGE_SECONDS],
        )
    except Exception:
        logging.debug("Failed to sweep stale batch progress", exc_info=True)
