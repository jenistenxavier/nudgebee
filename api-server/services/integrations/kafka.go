package integrations

import (
	"fmt"
	"nudgebee/services/integrations/core"
	"nudgebee/services/relay"
	"nudgebee/services/security"
	"strings"
)

func init() {
	core.RegisterIntegration(Kafka{})
}

const IntegrationKafka = "kafka"

type Kafka struct {
}

func (m Kafka) Name() string {
	return IntegrationKafka
}

func (m Kafka) Category() core.IntegrationCategory {
	return core.IntegrationCategoryMessagingQueue
}

func (m Kafka) ConfigSchema() core.IntegrationSchema {
	return core.IntegrationSchema{
		Type:     core.ToolSchemaTypeObject,
		Testable: true,
		Required: []string{"k8s_secret"},
		Properties: map[string]core.IntegrationSchemaProperty{
			"k8s_secret": {
				Type: core.ToolSchemaTypeString,
				Description: "Kafka credentials Secret in k8s. Required key: KAFKA_BROKERS (comma-separated host:port bootstrap brokers). " +
					"Optional keys for authenticated/encrypted clusters: KAFKA_SECURITY_PROTOCOL (PLAINTEXT|SSL|SASL_PLAINTEXT|SASL_SSL), " +
					"KAFKA_SASL_MECHANISM (PLAIN|SCRAM-SHA-256|SCRAM-SHA-512), KAFKA_SASL_USERNAME, KAFKA_SASL_PASSWORD",
				IsTestable: true,
				Priority:   82,
			},
			"account_id": {
				Type:             core.ToolSchemaTypeArray,
				Description:      "Select Account",
				Default:          "",
				AutoGenerateFunc: "listAccounts",
				Priority:         95,
			},
			"integration_config_name": {
				Type:             core.ToolSchemaTypeString,
				Description:      "Name of Kafka cluster",
				Default:          "",
				AutoGenerateFunc: "",
				Priority:         100,
			},
		},
	}
}

func (m Kafka) ValidateConfig(sc *security.SecurityContext, configs []core.IntegrationConfigValue, accountId string) []error {

	if accountId == "" {
		return []error{fmt.Errorf("account_id is required")}
	}

	secretName := ""
	for _, integrationConfig := range configs {
		if strings.EqualFold(integrationConfig.Name, "k8s_secret") {
			secretName = integrationConfig.Value
			break
		}
	}

	if secretName == "" {
		return []error{fmt.Errorf("k8s_secret is required")}
	}

	// Reject a namespace-prefixed secret ("namespace/secret"). relay.CommandExecutor would
	// otherwise run the test pod against a secret in that namespace; the kafka creds secret is
	// expected to live in the agent's own namespace, so a prefix should not be accepted here.
	if strings.Contains(secretName, "/") {
		return []error{fmt.Errorf("k8s_secret must be a secret name without a namespace prefix")}
	}

	// Fetch cluster metadata (brokers + topics) as the connection test. The kcat argv is built
	// via positional params so every secret value is double-quoted, keeping word-splitting /
	// shell metacharacters in (e.g.) a SASL password from breaking the command. SASL/TLS flags
	// are appended only when the matching secret key is set, so the same command works for
	// PLAINTEXT and SASL_SSL clusters alike.
	//
	// The final `2>&1 || true` redirects kcat's stderr (where it writes connection/auth errors)
	// into stdout and forces a zero exit, so the relay returns the output for the error-pattern
	// parsing below instead of surfacing a bare non-zero-exit failure. Connection problems are
	// still reported — they're matched from the captured text, not the exit code.
	command := `set -- -b "$KAFKA_BROKERS"; ` +
		`[ -n "$KAFKA_SECURITY_PROTOCOL" ] && set -- "$@" -X "security.protocol=$KAFKA_SECURITY_PROTOCOL"; ` +
		`[ -n "$KAFKA_SASL_MECHANISM" ] && set -- "$@" -X "sasl.mechanism=$KAFKA_SASL_MECHANISM"; ` +
		`[ -n "$KAFKA_SASL_USERNAME" ] && set -- "$@" -X "sasl.username=$KAFKA_SASL_USERNAME"; ` +
		`[ -n "$KAFKA_SASL_PASSWORD" ] && set -- "$@" -X "sasl.password=$KAFKA_SASL_PASSWORD"; ` +
		`kcat "$@" -L 2>&1 || true`
	// Inject the secret's keys as env vars into the script-runner pod. The llm-server MSSQL/Oracle
	// relay jobs populate this map explicitly for the same pod_script_run_enricher action, so do the
	// same here rather than rely on implicit wholesale injection. Keys map to themselves because the
	// secret key names and the env var names referenced in the command above are identical.
	envFromSecret := map[string]string{
		"KAFKA_BROKERS":           "KAFKA_BROKERS",
		"KAFKA_SECURITY_PROTOCOL": "KAFKA_SECURITY_PROTOCOL",
		"KAFKA_SASL_MECHANISM":    "KAFKA_SASL_MECHANISM",
		"KAFKA_SASL_USERNAME":     "KAFKA_SASL_USERNAME",
		"KAFKA_SASL_PASSWORD":     "KAFKA_SASL_PASSWORD",
	}
	resp, err := relay.CommandExecutor(accountId, command, secretName, envFromSecret)

	if err != nil {
		return core.HandleRelayError(err)
	}

	if resp == nil {
		return []error{fmt.Errorf("empty response from kafka server")}
	}
	respStr, ok := resp["response"].(string)
	if !ok {
		return []error{fmt.Errorf("unexpected response format from kafka server: %v", resp)}
	}

	respLower := strings.ToLower(respStr)

	// `kcat -L` prints the exact header "Metadata for all topics" only on a successful connection.
	// Match the full header (not a bare "metadata for") so a per-topic error such as
	// "Failed to acquire metadata for topic ..." cannot false-pass. Check this positive signal
	// before the error patterns below: the response lists every topic name, so matching error
	// keywords first could false-fail a healthy cluster hosting a topic whose name contains one
	// (e.g. a topic named "timeout" or "certificate", or "broker" matching "broker transport
	// failure"). A partially-degraded cluster still prints this header, which is correct — metadata
	// was acquired, so the connection works.
	if strings.Contains(respLower, "metadata for all topics") {
		return nil
	}

	// No success marker: map specific kcat/Kafka error patterns to actionable feedback.
	switch {
	case strings.Contains(respLower, "authentication failed") || strings.Contains(respLower, "sasl authentication"):
		return []error{fmt.Errorf("authentication failed: invalid SASL username or password")}
	case strings.Contains(respLower, "connection refused"):
		return []error{fmt.Errorf("connection refused: verify KAFKA_BROKERS host:port is correct and reachable")}
	case strings.Contains(respLower, "name or service not known") || strings.Contains(respLower, "no such host") ||
		strings.Contains(respLower, "could not resolve"):
		return []error{fmt.Errorf("host not found: verify the broker hostnames in KAFKA_BROKERS")}
	case strings.Contains(respLower, "timed out") || strings.Contains(respLower, "timeout"):
		return []error{fmt.Errorf("connection timed out: verify the Kafka brokers are reachable")}
	case strings.Contains(respLower, "ssl handshake") || strings.Contains(respLower, "certificate"):
		return []error{fmt.Errorf("TLS handshake failed: check KAFKA_SECURITY_PROTOCOL and CA/cert configuration")}
	case strings.Contains(respLower, "topic authorization failed") || strings.Contains(respLower, "not authorized"):
		return []error{fmt.Errorf("authorization failed: the user lacks permission to read cluster metadata")}
	}

	// Surface any other error indicators.
	if strings.Contains(respLower, "exit status") || strings.Contains(respLower, "error") {
		return []error{fmt.Errorf("kafka validation failed: %s", respStr)}
	}

	return []error{fmt.Errorf("failed to validate kafka connection - unexpected response: %s", respStr)}
}
