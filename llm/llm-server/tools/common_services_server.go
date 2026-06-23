package tools

import (
	"errors"
	"fmt"
	"log/slog"
	"nudgebee/llm/common"
	"nudgebee/llm/config"
	"nudgebee/llm/security"
	"nudgebee/llm/services_server"
	"nudgebee/llm/tools/core"
	"strconv"
	"strings"
	"time"
)

// lokiMaxLogLimit is the per-query cap Loki enforces (HTTP 400 above this).
// Sourced from the production error string `loki: limit exceeds maximum of 5000`
// (services-server wrapped) — pin here so the clamp does not lag the upstream
// reality. If Loki raises its cap, bump this constant.
const lokiMaxLogLimit = 5000

// clampLogLimitForProvider returns the (possibly capped) limit + a flag that
// indicates whether a clamp actually happened. Returns (limit, false) for
// providers without a known cap so the caller logs nothing in the common path.
// Provider name is normalised (trimmed + lowercased) so accidental whitespace
// in integration config doesn't bypass the clamp.
func clampLogLimitForProvider(provider string, limit int) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "loki":
		if limit > lokiMaxLogLimit {
			return lokiMaxLogLimit, true
		}
	}
	return limit, false
}

func executeFetchLogs(ctx core.NbToolContext, logProvider services_server.ObservabilityProvider, query string, configs map[string]any) (core.ObservabilityLogResponse, error) {
	if logProvider.Provider == "" {
		return core.ObservabilityLogResponse{}, errors.New("log_provider is required")
	}
	limit := 1000
	if val, ok := configs["limit"]; ok {
		switch limitValue := val.(type) {
		case string:
			limit1, err := strconv.Atoi(limitValue)
			if err != nil {
				return core.ObservabilityLogResponse{}, err
			} else {
				limit = limit1
			}
		case float64:
			limit = int(limitValue)
		case int:
			limit = limitValue
		case int64:
			limit = int(limitValue)
		default:
			return core.ObservabilityLogResponse{}, fmt.Errorf("invalid limit value - %v", val)

		}
	}

	// Defensive per-provider limit clamp. Some backends reject above their
	// own ceiling (Loki: HTTP 400 `limit exceeds maximum of 5000`); rather
	// than letting that surface to the LLM as an opaque error, cap the
	// request and log it. A capped fetch is strictly more useful than a
	// failed one. Pre-2026-06-22 a stale prompt at agent_log.go:264
	// instructed agents to request `limit 10000` for second-pass widened
	// fetches — the prompt has since been corrected to 5000, but the clamp
	// here also covers any other caller (manual integration, future agent,
	// raw API hit) that asks above the cap.
	if newLimit, clamped := clampLogLimitForProvider(logProvider.Provider, limit); clamped {
		slog.Warn("executeFetchLogs: clamped limit to provider backend cap", "provider", logProvider.Provider, "requested", limit, "capped_to", newLimit)
		limit = newLimit
	}
	endTime := int64(time.Now().UnixMilli())
	if val, ok := configs["end_time"]; ok {
		if intVal, ok := val.(int64); ok {
			endTime = intVal
		} else {
			return core.ObservabilityLogResponse{}, fmt.Errorf("invalid end_time value - %v", val)
		}
	}
	startTime := int64(time.Now().Add(-1 * time.Hour).UnixMilli())
	if val, ok := configs["start_time"]; ok {
		if intVal, ok := val.(int64); ok {
			startTime = intVal
		} else {
			return core.ObservabilityLogResponse{}, fmt.Errorf("invalid start_time value - %v", val)
		}
	}

	// Guard: if startTime >= endTime (LLM can produce same/inverted timestamps),
	// default to a 1-hour window ending at endTime.
	if startTime >= endTime {
		slog.Warn("executeFetchLogs: startTime >= endTime, defaulting to 1h window", "startTime", startTime, "endTime", endTime)
		startTime = endTime - time.Hour.Milliseconds()
	}

	offset := 0
	if val, ok := configs["offset"]; ok {
		switch offsetValue := val.(type) {
		case string:
			offsetParsed, err := strconv.Atoi(offsetValue)
			if err != nil {
				return core.ObservabilityLogResponse{}, err
			} else {
				offset = offsetParsed
			}
		case float64:
			offset = int(offsetValue)
		case int:
			offset = offsetValue
		case int64:
			offset = int(offsetValue)
		default:
			return core.ObservabilityLogResponse{}, fmt.Errorf("invalid offset value - %v", val)
		}
	}
	request := map[string]any{}
	if val, ok := configs["request"]; ok {
		if valMap, ok := val.(map[string]any); ok {
			request = valMap
		} else {
			return core.ObservabilityLogResponse{}, fmt.Errorf("invalid request value - %v", val)
		}
	}

	index := ""
	if val, ok := configs["index"]; ok {
		if s, ok := val.(string); ok {
			index = s
		}
	}

	// Leave an empty source empty: api-server re-resolves it from the account's
	// integration. Forcing "agent" here would pin SaaS providers to the wrong backend.

	logRequest := services_server.LogQueryRequest{
		Query:             query,
		Limit:             limit,
		StartTime:         startTime,
		EndTime:           endTime,
		AccountId:         ctx.AccountId,
		LogProvider:       logProvider.Provider,
		LogProviderSource: logProvider.IntegrationSource,
		Offset:            offset,
		Request:           request,
		Index:             index,
	}
	logs, err := services_server.QueryLogs(*ctx.Ctx, logRequest)
	if err != nil {
		return core.ObservabilityLogResponse{}, err
	}
	return logs, nil
}

func executeFetchLogLabels(accountId string, logProvider services_server.ObservabilityProvider) (core.ObservabilityLogLabelResponse, error) {
	if logProvider.Provider == "" {
		return core.ObservabilityLogLabelResponse{}, errors.New("log_provider is required")
	}

	tenantId, err := security.GetTenantIdFromAccountId(accountId)
	if err != nil {
		return core.ObservabilityLogLabelResponse{}, err
	}
	ctx := security.NewRequestContextForTenantAccountAdmin(tenantId, "", []string{accountId})

	labels, err := services_server.QueryLogLabels(*ctx, accountId, logProvider)
	if err != nil {
		return core.ObservabilityLogLabelResponse{}, err
	}
	return labels, nil
}

func getProvider(accountId, providerType string) (services_server.ObservabilityProvider, error) {
	if accountId == "" || providerType == "" {
		return services_server.ObservabilityProvider{}, fmt.Errorf("accountId or providerType cannot be empty")
	}

	tenantId, err := security.GetTenantIdFromAccountId(accountId)
	if err != nil {
		return services_server.ObservabilityProvider{}, err
	}
	securityContext := security.NewRequestContextForTenantAccountAdmin(tenantId, "", []string{accountId})
	provider, err := services_server.GetObservabilityProvider(*securityContext, accountId, providerType)
	if err != nil {
		return services_server.ObservabilityProvider{}, err
	}
	return provider, nil
}

func executeFetchTrace(ctx core.NbToolContext, traceProvider string, traceProviderSource string, query string, queryBuilder core.TraceQueryBuilder, config map[string]any) (core.ObservabilityTraceResponse, error) {
	limit := 1000
	if val, ok := config["limit"]; ok {
		switch limitValue := val.(type) {
		case string:
			limit1, err := strconv.Atoi(limitValue)
			if err != nil {
				return core.ObservabilityTraceResponse{}, err
			} else {
				limit = limit1
			}
		case float64:
			limit = int(limitValue)
		case int:
			limit = limitValue
		case int64:
			limit = int(limitValue)
		default:
			return core.ObservabilityTraceResponse{}, fmt.Errorf("invalid limit value - %v", val)

		}
	}
	endTime := int64(time.Now().UnixMilli())
	if val, ok := config["end_time"]; ok {
		if intVal, ok := val.(int64); ok {
			endTime = intVal
		} else {
			return core.ObservabilityTraceResponse{}, fmt.Errorf("invalid end_time value - %v", val)
		}
	}
	startTime := int64(time.Now().Add(-1 * time.Hour).UnixMilli())
	if val, ok := config["start_time"]; ok {
		if intVal, ok := val.(int64); ok {
			startTime = intVal
		} else {
			return core.ObservabilityTraceResponse{}, fmt.Errorf("invalid start_time value - %v", val)
		}
	}

	offset := 0
	if val, ok := config["offset"]; ok {
		switch offsetValue := val.(type) {
		case string:
			offsetParsed, err := strconv.Atoi(offsetValue)
			if err != nil {
				return core.ObservabilityTraceResponse{}, err
			} else {
				offset = offsetParsed
			}
		case float64:
			offset = int(offsetValue)
		case int:
			offset = offsetValue
		case int64:
			offset = int(offsetValue)
		default:
			return core.ObservabilityTraceResponse{}, fmt.Errorf("invalid offset value - %v", val)
		}
	}
	queryBuilder.Offset = offset
	queryBuilder.Limit = limit
	traceRequest := core.ObservabilityTracesV3Request{
		AccountId:      ctx.AccountId,
		ProviderType:   traceProvider,
		ProviderSource: traceProviderSource,
		StartTime:      startTime,
		EndTime:        endTime,
		Limit:          limit,
		Offset:         offset,
		QueryRequest:   queryBuilder,
		Query:          query,
		// Only the free-form ClickHouse SQL path (traces_execute) asks for the raw column/row
		// table, so aggregation / custom-projection queries keep their real values instead of
		// being zeroed by the fixed span-schema mapping. Other providers and the structured
		// query-builder path keep the typed span array.
		IncludeRawResult: traceProvider == "otel_clickhouse" && query != "",
	}
	traces, err := services_server.QueryTraces(*ctx.Ctx, traceRequest)
	if err != nil {
		return core.ObservabilityTraceResponse{}, err
	}
	return traces, nil
}

func GetTraceProvider(accountId string) (services_server.ObservabilityProvider, error) {
	providerFromServicesServer, err := getProvider(accountId, "traces")
	if err == nil {
		if providerFromServicesServer.Provider != "" {
			return providerFromServicesServer, nil
		}
	}

	if err != nil {
		slog.Warn("trace: could not fetch provider from services-server, falling back to default provider", "error", err, "accountId", accountId)
	}
	traceProvider := "clickhouse"
	return services_server.ObservabilityProvider{
		IntegrationSource: "agent",
		Provider:          traceProvider,
	}, nil
}

func GetMetricsProvider(accountId string) (services_server.ObservabilityProvider, error) {
	metricsConnectionProvider := "prometheus"
	providerFromServicesServer, err := getProvider(accountId, "metrics")
	if err == nil {
		if providerFromServicesServer.Provider != "" {
			return providerFromServicesServer, nil
		}
	}

	if err != nil {
		slog.Warn("metrics: could not fetch provider from services-server, falling back to local DB", "error", err, "accountId", accountId)
	}

	return services_server.ObservabilityProvider{
		Provider:          metricsConnectionProvider,
		IntegrationSource: "agent",
	}, nil
}

func GetLogProvider(accountId string) (services_server.ObservabilityProvider, error) {
	// Dev-only override: bypass per-account routing. Normalized for common
	// operator fumbles (case, whitespace). No allowlist — the authoritative
	// provider set lives in api-server's dispatch table; mirroring would
	// drift. Unknown values surface via this Warn and fail on the next query.
	if override := strings.ToLower(strings.TrimSpace(config.Config.LLMServerLogProviderOverride)); override != "" {
		slog.Warn("logs: using LLM_SERVER_LOG_PROVIDER_OVERRIDE — bypasses per-account routing; only intended for local dev / debugging",
			"provider", override,
			"raw", config.Config.LLMServerLogProviderOverride,
			"accountId", accountId)
		return services_server.ObservabilityProvider{
			Provider:          override,
			IntegrationSource: "agent",
		}, nil
	}

	logConnectionProvider := "k8s"
	providerFromServicesServer, err := getProvider(accountId, "logs")
	if err == nil {
		if providerFromServicesServer.Provider != "" {
			return providerFromServicesServer, nil
		}
	}

	if err != nil {
		slog.Warn("logs: could not fetch provider from services-server, falling back to local DB", "error", err, "accountId", accountId)
	}

	// Fallback to fetching from DB
	dbms, err := common.GetDatabaseManager(common.Metastore)
	if err != nil {
		slog.Error("logs: unable to fetch dbms", "error", err)
		return services_server.ObservabilityProvider{}, err
	}
	rows, err := dbms.Db.Queryx("select connection_status::text from agent where cloud_account_id = $1", accountId)
	if err != nil {
		slog.Error("logs: unable to fetch dbms", "error", err)
		return services_server.ObservabilityProvider{}, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("logs: failed to close rows", "error", err)
		}
	}()

	for rows.Next() {
		var connectionStatusString *string
		err := rows.Scan(&connectionStatusString)
		if err != nil {
			slog.Error("logs: unable to scan rows", "error", err)
			break
		}
		connectionStatus := map[string]any{}
		if connectionStatusString != nil {
			err = common.UnmarshalJson([]byte(*connectionStatusString), &connectionStatus)
			if err != nil {
				slog.Error("logs: unable to unmarshal rows", "error", err)
				break
			}
		}
		logConnectionProvider1 := connectionStatus["logsConnectionProvider"]
		if logConnectionProvider1 != nil {
			logConnectionProvider = logConnectionProvider1.(string)
		} else {
			slog.Info("logs: unable to find log connection provider, will be using default")
		}
	}

	return services_server.ObservabilityProvider{Provider: logConnectionProvider, IntegrationSource: "agent"}, nil
}

func HasDatadogIntegration(accountId string) bool {
	if accountId == "" {
		return false
	}

	// Query database for Datadog integration
	dbms, err := common.GetDatabaseManager(common.Metastore)
	if err != nil {
		slog.Error("HasDatadogIntegration: unable to get database manager", "error", err)
		return false
	}

	// Check if account has Datadog integration configured
	// Based on pattern from tools/tool_docs.go:204 and agents/core/llm_common.go:1443
	query := `
		SELECT COUNT(*)
		FROM integrations i
		JOIN integrations_cloud_accounts ia ON i.id = ia.integration_id
		WHERE i.type = 'datadog' AND ia.cloud_account_id = $1 and i.status = 'enabled'
	`

	var count int
	err = dbms.Db.Get(&count, query, accountId)
	if err != nil {
		slog.Warn("HasDatadogIntegration: error querying integrations", "error", err, "accountId", accountId)
		return false
	}

	return count > 0
}
