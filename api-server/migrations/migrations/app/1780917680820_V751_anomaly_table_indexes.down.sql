DROP INDEX IF EXISTS public.idx_anomaly_account_type;
DROP INDEX IF EXISTS public.idx_anomaly_account_evaluated_at;
DROP INDEX IF EXISTS public.idx_anomaly_open_account_type_name;

-- Restore the partial index removed by the up migration.
CREATE INDEX IF NOT EXISTS idx_anomaly_status_type
    ON public.anomaly (anomaly_status, anomaly_type)
    WHERE anomaly_status = 'OPEN';
