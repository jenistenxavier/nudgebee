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

	// Fetch cluster metadata (brokers + topics) as the connection test. The kcat argv is built
	// via positional params so every secret value is double-quoted, keeping word-splitting /
	// shell metacharacters in (e.g.) a SASL password from breaking the command. SASL/TLS flags
	// are appended only when the matching secret key is set, so the same command works for
	// PLAINTEXT and SASL_SSL clusters alike.
	command := `set -- -b "$KAFKA_BROKERS"; ` +
		`[ -n "$KAFKA_SECURITY_PROTOCOL" ] && set -- "$@" -X "security.protocol=$KAFKA_SECURITY_PROTOCOL"; ` +
		`[ -n "$KAFKA_SASL_MECHANISM" ] && set -- "$@" -X "sasl.mechanism=$KAFKA_SASL_MECHANISM"; ` +
		`[ -n "$KAFKA_SASL_USERNAME" ] && set -- "$@" -X "sasl.username=$KAFKA_SASL_USERNAME"; ` +
		`[ -n "$KAFKA_SASL_PASSWORD" ] && set -- "$@" -X "sasl.password=$KAFKA_SASL_PASSWORD"; ` +
		`kcat "$@" -L`
	resp, err := relay.CommandExecutor(accountId, command, secretName, map[string]string{})

	if err != nil {
		return core.HandleRelayError(err)
	}

	respStr, ok := resp["response"].(string)
	if !ok {
		return []error{fmt.Errorf("unexpected response format from kafka server: %v", resp)}
	}

	respLower := strings.ToLower(respStr)

	// Check for specific Kafka/kcat error patterns to provide actionable feedback.
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

	// `kcat -L` prints "Metadata for all topics" only on a successful connection. Check this
	// positive signal before the generic error check: a loose "broker" match would false-pass
	// on errors like "broker transport failure" / "all brokers down", while checking for "error"
	// first would false-fail a healthy cluster that happens to host a topic named "*error*".
	if strings.Contains(respLower, "metadata for") {
		return nil
	}

	// Surface any other error indicators.
	if strings.Contains(respLower, "exit status") || strings.Contains(respLower, "error") {
		return []error{fmt.Errorf("kafka validation failed: %s", respStr)}
	}

	return []error{fmt.Errorf("failed to validate kafka connection - unexpected response: %s", respStr)}
}
