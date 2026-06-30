package flow_sources

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"nudgebee/services/knowledge_graph/core"
)

// fixtureSpan mirrors the relevant fields of a collector trace span in the saved
// query_traces response fixture.
type fixtureSpan struct {
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id"`
	Name         string            `json:"name"`
	DurationMs   float64           `json:"duration_ms"`
	Labels       map[string]string `json:"labels"`
}

type fixtureResponse struct {
	Data struct {
		Spans []fixtureSpan `json:"spans"`
	} `json:"data"`
}

// loadFixtureSpans loads the GCP query_traces fixture (shared with the cloud package)
// and projects it into the local gcpTraceSpan shape.
func loadFixtureSpans(t *testing.T) []gcpTraceSpan {
	t.Helper()
	path := filepath.Join("..", "..", "cloud", "testdata", "query_traces_gcp_response.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	var resp fixtureResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if len(resp.Data.Spans) == 0 {
		t.Fatalf("fixture has no spans")
	}
	spans := make([]gcpTraceSpan, 0, len(resp.Data.Spans))
	for _, s := range resp.Data.Spans {
		spans = append(spans, gcpTraceSpan{
			SpanID:       s.SpanID,
			ParentSpanID: s.ParentSpanID,
			Name:         s.Name,
			DurationMs:   s.DurationMs,
			Labels:       s.Labels,
		})
	}
	return spans
}

func TestBuildGCPServiceMap_FromFixture(t *testing.T) {
	spans := loadFixtureSpans(t)
	sm := buildGCPServiceMap(spans)
	if sm == nil || len(sm.Applications) == 0 {
		t.Fatalf("expected a non-empty service map")
	}

	apps := make(map[string]bool)
	for _, app := range sm.Applications {
		if app.Id.Kind != "ServerlessFunction" {
			t.Errorf("app %q: kind = %q, want ServerlessFunction", app.Id.Name, app.Id.Kind)
		}
		apps[app.Id.Name] = true
	}
	// The full-auth project's Cloud Run services.
	if !apps["oauth"] {
		t.Errorf("expected an 'oauth' application; got %v", apps)
	}

	// oauth must carry a Redis dependency (it hits redis heavily in the fixture).
	redisReq, hasRedis, upstreams := -1.0, false, 0
	for _, app := range sm.Applications {
		if app.Id.Name != "oauth" {
			continue
		}
		upstreams = len(app.Upstreams)
		for _, up := range app.Upstreams {
			if up.Id == formatUpstreamID("ExternalService", "Redis") {
				hasRedis = true
				redisReq = up.RequestCount
			}
		}
	}
	if upstreams == 0 {
		t.Fatalf("oauth has no upstreams")
	}
	if !hasRedis {
		t.Errorf("oauth missing Redis upstream")
	}
	if redisReq <= 0 {
		t.Errorf("oauth→Redis RequestCount = %v, want > 0", redisReq)
	}
}

func TestConvertGCPServiceMap_EmitsCallsEdges(t *testing.T) {
	spans := loadFixtureSpans(t)
	sm := buildGCPServiceMap(spans)

	base := NewBaseFlowSource(gcpTracesSourceName, core.FlowSourceCategoryTracing, true, slog.Default())
	// No matcher initialized → converter creates fresh nodes for every endpoint.
	nodes, edges, err := ConvertServiceMapToGraph(sm, "acct-1", "tenant-1", base, slog.Default(), gcpTracesSourceName)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(edges) == 0 {
		t.Fatalf("expected CALLS edges from the converted service map")
	}

	byID := make(map[string]*core.DbNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	nodeName := func(id string) string {
		if n, ok := byID[id]; ok {
			if v, ok := n.Properties["name"].(string); ok {
				return v
			}
		}
		return ""
	}

	redisInbound := false
	for _, e := range edges {
		if e.Source != gcpTracesSourceName {
			t.Errorf("edge source = %q, want %q", e.Source, gcpTracesSourceName)
		}
		if e.RelationshipType != core.RelationshipCalls {
			t.Errorf("edge relationship = %q, want CALLS", e.RelationshipType)
		}
		if nodeName(e.DestinationNodeID) == "Redis" {
			redisInbound = true
		}
	}
	if !redisInbound {
		t.Errorf("expected at least one CALLS edge into the Redis ExternalService node")
	}

	// The Redis dependency must materialize as an ExternalService node.
	redisIsExternal := false
	for _, n := range nodes {
		if name, _ := n.Properties["name"].(string); name == "Redis" && n.NodeType == core.NodeTypeExternalService {
			redisIsExternal = true
		}
	}
	if !redisIsExternal {
		t.Errorf("expected a Redis ExternalService node")
	}
}
