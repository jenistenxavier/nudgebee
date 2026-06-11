package api

import (
	"testing"

	"nudgebee/services/security"
)

// TestRequiredPermission_FailClosed locks the relay authorization contract:
// only enumerated read action_names map to Read; everything else (mutations,
// exec, job-creating actions, and crucially anything not yet enumerated) must
// require write access so a read-only role can never invoke it.
func TestRequiredPermission_FailClosed(t *testing.T) {
	reads := []string{
		"get_resource",
		"get_resource_yaml",
		"metrics",
		"logs",
		"traces",
		"service_map",
		"pod_metric_enricher",
		"datadog_incident",
	}
	for _, a := range reads {
		if got := requiredPermission(a); got != security.SecurityAccessTypeRead {
			t.Errorf("requiredPermission(%q) = %q, want Read", a, got)
		}
	}

	// Known mutating relay actions (must require write) plus an unknown/typo
	// action that must fail closed rather than default to read.
	writes := []string{
		"replica_rightsizing",
		"replica_right_sizing",
		"rightsize_pvc",
		"rightsizing_resource",
		"volume_delete",
		"continuous_rightsizing",
		"delete_pod",
		"replace_workload",
		"kubectl_command_executor",
		"pod_bash_enricher",
		"nubi_enricher",
		"image_scanner",
		"pod_profiler",
		"",                        // empty action_name
		"some_unenumerated_write", // future action nobody added to readActions
	}
	for _, a := range writes {
		if got := requiredPermission(a); got != security.SecurityAccessTypeUpdate {
			t.Errorf("requiredPermission(%q) = %q, want Update (fail-closed)", a, got)
		}
	}
}
