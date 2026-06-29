package recommendation

import (
	"encoding/json"
	"strings"

	"nudgebee/services/knowledge_graph/core"
)

// k8sRecRef is the minimal identity extracted from a recommendation needed to
// locate its workload in the knowledge graph.
type k8sRecRef struct {
	Namespace string
	Workload  string
}

// recommendationImpact bundles the safety band plus a compact impact summary,
// shaped for embedding in a recommendation's finops_score_breakdown JSONB.
type recommendationImpact struct {
	Band    SafetyBand
	Reason  string
	Summary map[string]any
}

// extractK8sRecRef pulls namespace + workload name from a recommendation's JSONB
// payload, tolerating the key variations across rule types (controller_name,
// controllerName, a nested workload{} object, or a bare name). Returns ok=false
// when identity is incomplete, so the caller degrades to an "unknown" safety band
// rather than guessing.
func extractK8sRecRef(rec map[string]any) (k8sRecRef, bool) {
	ns := firstNonEmptyString(rec, "namespace")

	// Some rules (e.g. health_check) nest a workload object.
	if wl, ok := rec["workload"].(map[string]any); ok {
		if ns == "" {
			ns = firstNonEmptyString(wl, "namespace")
		}
		if name := firstNonEmptyString(wl, "name"); ns != "" && name != "" {
			return k8sRecRef{Namespace: ns, Workload: name}, true
		}
	}

	name := firstNonEmptyString(rec, "controller_name", "controllerName", "workload_name", "name")
	if ns != "" && name != "" {
		return k8sRecRef{Namespace: ns, Workload: name}, true
	}
	return k8sRecRef{}, false
}

// firstNonEmptyString returns the first key whose value is a non-blank string.
func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// resolveK8sWorkloadNodeID finds the knowledge-graph Workload node for a k8s
// recommendation by (account, namespace, name). Scoping to the recommendation's
// cloud account disambiguates the same namespace/name living in two k8s accounts
// (e.g. dev and prod clusters); the recommendation belongs to exactly one. Any
// remaining ambiguity is treated as unresolved rather than guessed.
func resolveK8sWorkloadNodeID(kg *core.Service, tenantID, accountID string, ref k8sRecRef) (string, bool) {
	params := core.SearchNodesParams{
		Name:      ref.Workload,
		Namespace: ref.Namespace,
		NodeTypes: []core.NodeType{core.NodeTypeWorkload},
		Limit:     2,
	}
	if accountID != "" {
		params.AccountIDs = []string{accountID}
	}
	res, err := kg.SearchNodes(tenantID, params)
	if err != nil || res == nil || len(res.Nodes) != 1 {
		return "", false
	}
	return res.Nodes[0].ID, true
}

// annotateK8sRecommendationImpact resolves a k8s recommendation to its workload
// node, computes the blast radius, and derives the safety band. ok=false means
// the recommendation could not be resolved (caller should leave it unannotated).
func annotateK8sRecommendationImpact(kg *core.Service, tenantID, accountID string, recPayload map[string]any) (recommendationImpact, bool) {
	ref, ok := extractK8sRecRef(recPayload)
	if !ok {
		return recommendationImpact{}, false
	}
	nodeID, ok := resolveK8sWorkloadNodeID(kg, tenantID, accountID, ref)
	if !ok {
		return recommendationImpact{}, false
	}
	impact, err := kg.GetImpactedServices(tenantID, nodeID, nil, 2)
	if err != nil {
		return recommendationImpact{}, false
	}
	band, reason := DeriveSafetyBand(impact)
	return recommendationImpact{
		Band:   band,
		Reason: reason,
		Summary: map[string]any{
			"dependent_count":       impact.DependentCount,
			"production_dependents": impact.ProductionDependents,
			"coverage_confidence":   string(impact.CoverageConfidence),
			"truncated":             impact.Truncated,
			"safety_reason":         reason,
		},
	}, true
}

// annotateBreakdownWithImpact resolves a k8s recommendation's blast radius and
// stamps safety_band + impact_summary into the score breakdown map. It is a no-op
// when the recommendation can't be resolved (non-k8s, ambiguous, or absent from
// the graph). Results are memoized per workload via cache.
func annotateBreakdownWithImpact(kg *core.Service, tenantID, accountID string, recJSON []byte, breakdown map[string]any, cache map[string]*recommendationImpact) {
	if kg == nil || breakdown == nil {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(recJSON, &payload); err != nil {
		return
	}
	ref, ok := extractK8sRecRef(payload)
	if !ok {
		return
	}
	key := tenantID + "|" + accountID + "|" + ref.Namespace + "|" + ref.Workload
	imp, seen := cache[key]
	if !seen {
		// Cache the negative too (nil): a workload that can't be resolved once
		// won't resolve for the next recommendation either, so don't re-run the
		// search + traversal for every rec on the same unresolved workload.
		if resolved, ok := annotateK8sRecommendationImpact(kg, tenantID, accountID, payload); ok {
			imp = &resolved
		}
		cache[key] = imp
	}
	if imp == nil {
		return
	}
	breakdown["safety_band"] = string(imp.Band)
	breakdown["impact_summary"] = imp.Summary
}
