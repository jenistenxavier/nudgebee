package sources

import (
	"encoding/json"
	"log/slog"
	"nudgebee/services/knowledge_graph/core"
	"os"
	"testing"
)

func TestGCPSourceGenerateUniqueKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	source, err := NewGCPSource(GCPSourceConfig{}, logger)
	if err != nil {
		t.Fatalf("Failed to create GCPSource: %v", err)
	}

	tests := []struct {
		name    string
		node    *core.DbNode
		wantKey string
	}{
		{
			name: "ComputeInstance node",
			node: &core.DbNode{
				NodeType: core.NodeTypeComputeInstance,
				Properties: map[string]interface{}{
					"name":   "web-server-1",
					"region": "us-central1",
				},
				CloudAccountID: "acc-1",
			},
			wantKey: "gcp:acc-1:us-central1:ComputeInstance:web-server-1:web-server-1",
		},
		{
			name: "Database node with project_id",
			node: &core.DbNode{
				NodeType: core.NodeTypeDatabase,
				Properties: map[string]interface{}{
					"name":           "my-sql-instance",
					"region":         "us-central1",
					"gcp_project_id": "my-project",
				},
				CloudAccountID: "acc-1",
			},
			wantKey: "gcp:acc-1:us-central1:Database:my-sql-instance:my-sql-instance",
		},
		{
			name: "VPC node",
			node: &core.DbNode{
				NodeType: core.NodeTypeVPC,
				Properties: map[string]interface{}{
					"name":   "default",
					"region": "us-central1",
				},
				CloudAccountID: "acc-1",
			},
			wantKey: "gcp:acc-1:us-central1:VPC:default:default",
		},
		{
			name:    "Nil node returns empty",
			node:    nil,
			wantKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := source.GenerateUniqueKey(tt.node)
			if key != tt.wantKey {
				t.Errorf("GenerateUniqueKey() = %q, want %q", key, tt.wantKey)
			}

			// Deterministic
			if tt.node != nil {
				key2 := source.GenerateUniqueKey(tt.node)
				if key != key2 {
					t.Errorf("Not deterministic: %q != %q", key, key2)
				}
			}
		})
	}
}

func TestGCPSourceDetermineNodeType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	source, err := NewGCPSource(GCPSourceConfig{}, logger)
	if err != nil {
		t.Fatalf("Failed to create GCPSource: %v", err)
	}

	tests := []struct {
		name        string
		resType     string
		serviceName string
		want        core.NodeType
	}{
		{"compute-engine", "compute-engine", "Compute Engine", core.NodeTypeComputeInstance},
		{"compute asset", "compute.googleapis.com/Instance", "Compute Engine", core.NodeTypeComputeInstance},
		{"cloud-sql", "cloud-sql", "Cloud SQL", core.NodeTypeDatabase},
		{"sqladmin asset", "sqladmin.googleapis.com/Instance/POSTGRES_17", "Cloud SQL", core.NodeTypeDatabase},
		{"kubernetes-engine", "kubernetes-engine", "Kubernetes Engine", core.NodeTypeManagedCluster},
		{"gke asset", "container.googleapis.com/Cluster", "Kubernetes Engine", core.NodeTypeManagedCluster},
		{"bigquery", "bigquery", "BigQuery", core.NodeTypeDatabase},
		{"bq dataset", "bigquery.googleapis.com/Dataset", "BigQuery", core.NodeTypeDatabase},
		{"bq table", "bigquery.googleapis.com/Table", "BigQuery", core.NodeTypeDatabase},
		{"storage bucket", "storage.googleapis.com/Bucket", "Cloud Storage", core.NodeTypeStorage},
		{"networking", "networking", "Networking", core.NodeTypeVPC},
		{"subnet", "subnet", "Networking", core.NodeTypeSubnet},
		{"cloud-logging", "cloud-logging", "Cloud Logging", core.NodeTypeLogAggregator},
		{"cloud-monitoring", "cloud-monitoring", "Cloud Monitoring", core.NodeTypeMonitoringService},
		{"vertex-ai", "vertex-ai", "Vertex AI", core.NodeTypeAIService},
		{"gemini-api", "gemini-api", "Gemini API", core.NodeTypeAIService},
		{"url-map is a route table", "url-map", "Cloud Load Balancing", core.NodeTypeRouteTable},
		{"cloud function is serverless", "cloudfunctions.googleapis.com/Function", "Cloud Functions", core.NodeTypeServerlessFunction},
		{"unknown type fallback to service", "unknown-type", "Compute Engine", core.NodeTypeComputeInstance},
		{"completely unknown", "unknown-type", "Unknown Service", core.NodeTypeCloudResource},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := source.determineNodeType(tt.resType, tt.serviceName)
			if got != tt.want {
				t.Errorf("determineNodeType(%q, %q) = %q, want %q", tt.resType, tt.serviceName, got, tt.want)
			}
		})
	}
}

func TestExtractGCPShortName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"full path", "my-project/zones/us-central1-a/instances/web-server-1", "web-server-1"},
		{"simple name", "web-server-1", "web-server-1"},
		{"two parts", "project/instance-name", "instance-name"},
		{"empty", "", ""},
		{"trailing slash", "project/", "project"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractGCPShortName(tt.input)
			if got != tt.want {
				t.Errorf("extractGCPShortName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractGCPProjectID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"full path", "my-project/zones/us-central1-a/instances/foo", "my-project"},
		{"simple name", "simple-name", "simple-name"},
		{"two parts", "project/instance", "project"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractGCPProjectID(tt.input)
			if got != tt.want {
				t.Errorf("extractGCPProjectID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractGCPResourceNameFromURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"full URL",
			"https://www.googleapis.com/compute/v1/projects/my-project/global/networks/default",
			"default",
		},
		{
			"path only",
			"projects/my-project/regions/us-central1/subnetworks/default-subnet",
			"default-subnet",
		},
		{"simple name", "default", "default"},
		{"empty", "", ""},
		{
			"trailing slash",
			"https://www.googleapis.com/compute/v1/projects/my-project/global/networks/default/",
			"default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractGCPResourceNameFromURL(tt.input)
			if got != tt.want {
				t.Errorf("extractGCPResourceNameFromURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGCPSourceGetName(t *testing.T) {
	source, err := NewGCPSource(GCPSourceConfig{}, nil)
	if err != nil {
		t.Fatalf("Failed to create GCPSource: %v", err)
	}

	if got := source.GetName(); got != "gcp" {
		t.Errorf("GetName() = %q, want 'gcp'", got)
	}
}

func TestGCPSourceIsEnabled(t *testing.T) {
	source, err := NewGCPSource(GCPSourceConfig{}, nil)
	if err != nil {
		t.Fatalf("Failed to create GCPSource: %v", err)
	}

	if !source.IsEnabled() {
		t.Error("IsEnabled() = false, want true")
	}
}

func TestGCPSourceValidate(t *testing.T) {
	source, err := NewGCPSource(GCPSourceConfig{}, nil)
	if err != nil {
		t.Fatalf("Failed to create GCPSource: %v", err)
	}

	if err := source.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestGCPSourceShouldIncludeResource(t *testing.T) {
	source, err := NewGCPSource(GCPSourceConfig{
		ServiceTypeFilter: GCPDefaultServiceTypeFilter,
	}, nil)
	if err != nil {
		t.Fatalf("Failed to create GCPSource: %v", err)
	}

	tests := []struct {
		name     string
		resource CloudResourceRow
		want     bool
	}{
		{
			"allowed compute-engine type",
			CloudResourceRow{Type: "compute-engine", ServiceName: "Compute Engine"},
			true,
		},
		{
			"allowed asset inventory compute type",
			CloudResourceRow{Type: "compute.googleapis.com/Instance", ServiceName: "Compute Engine"},
			true,
		},
		{
			"disallowed type for filtered service",
			CloudResourceRow{Type: "some-unknown-type", ServiceName: "Compute Engine"},
			false,
		},
		{
			"unknown service passes through",
			CloudResourceRow{Type: "custom-thing", ServiceName: "Custom Service"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := source.shouldIncludeResource(&tt.resource)
			if got != tt.want {
				t.Errorf("shouldIncludeResource(%s/%s) = %v, want %v",
					tt.resource.Type, tt.resource.ServiceName, got, tt.want)
			}
		})
	}
}

func TestParseGCloudCLIResponse(t *testing.T) {
	tests := []struct {
		name    string
		resp    map[string]any
		wantErr bool
		wantVal string
	}{
		{
			"data field string",
			map[string]any{"data": `[{"name":"test"}]`},
			false,
			`[{"name":"test"}]`,
		},
		{
			"output field",
			map[string]any{"output": `[{"name":"test2"}]`},
			false,
			`[{"name":"test2"}]`,
		},
		{
			"result field",
			map[string]any{"result": `[{"name":"test3"}]`},
			false,
			`[{"name":"test3"}]`,
		},
		{
			"empty response",
			map[string]any{},
			true,
			"",
		},
		{
			"nil response",
			nil,
			true,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGCloudCLIResponse(tt.resp)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGCloudCLIResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantVal {
				t.Errorf("parseGCloudCLIResponse() = %q, want %q", got, tt.wantVal)
			}
		})
	}
}

func TestCreateSubnetToVPCEdges(t *testing.T) {
	source, err := NewGCPSource(GCPSourceConfig{}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatalf("Failed to create GCPSource: %v", err)
	}
	req := &core.SourceBuildRequest{TenantID: "t1", CloudAccountID: "acct-1"}

	newSubnet := func(name, vpcID string) *core.DbNode {
		props := map[string]interface{}{"name": name}
		if vpcID != "" {
			props["vpc_id"] = vpcID
		}
		return core.NewNode(core.NodeTypeSubnet, "gcp:acct-1:r:Subnet:"+name+":"+name, props, req.TenantID, req.CloudAccountID, "gcp")
	}
	newVPC := func(name string) *core.DbNode {
		return core.NewNode(core.NodeTypeVPC, "gcp:acct-1:global:VPC:"+name+":"+name,
			map[string]interface{}{"name": name}, req.TenantID, req.CloudAccountID, "gcp")
	}

	t.Run("single VPC links explicit and infers orphans", func(t *testing.T) {
		vpc := newVPC("default")
		matched := newSubnet("with-vpc", "default")
		orphan := newSubnet("no-vpc", "")
		// Explicit vpc_id pointing at a VPC absent from this account's lookup
		// (e.g. Shared VPC). Must NOT be inferred to the sole VPC.
		absent := newSubnet("absent-vpc", "host-shared-vpc")
		lookup := newNodeLookup([]*core.DbNode{vpc, matched, orphan, absent})

		edges := source.createSubnetToVPCEdges(nil, lookup, req)
		if len(edges) != 2 {
			t.Fatalf("expected 2 edges, got %d", len(edges))
		}
		byConn := map[string]int{}
		for _, e := range edges {
			if e.DestinationNodeID != vpc.ID {
				t.Errorf("edge does not point at the VPC node")
			}
			if e.SourceNodeID == absent.ID {
				t.Errorf("subnet with an explicit (absent) vpc_id must not be inferred to soleVPC")
			}
			byConn[e.Properties["connection_type"].(string)]++
		}
		if byConn["vpc"] != 1 || byConn["vpc_inferred"] != 1 {
			t.Errorf("expected one 'vpc' and one 'vpc_inferred' edge, got %v", byConn)
		}
	})

	t.Run("multiple VPCs: no inference for subnets without vpc_id", func(t *testing.T) {
		vpcA := newVPC("default")
		vpcB := newVPC("prod")
		orphan := newSubnet("no-vpc", "")
		matched := newSubnet("with-vpc", "prod")
		lookup := newNodeLookup([]*core.DbNode{vpcA, vpcB, orphan, matched})

		edges := source.createSubnetToVPCEdges(nil, lookup, req)
		if len(edges) != 1 {
			t.Fatalf("expected 1 edge (only the explicit match), got %d", len(edges))
		}
		if edges[0].Properties["connection_type"].(string) != "vpc" {
			t.Errorf("expected explicit 'vpc' edge, got %v", edges[0].Properties["connection_type"])
		}
		if edges[0].DestinationNodeID != vpcB.ID {
			t.Errorf("explicit match should point at the named VPC")
		}
	})
}

func TestExtractGCPSubnetMetadata(t *testing.T) {
	source, err := NewGCPSource(GCPSourceConfig{}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatalf("Failed to create GCPSource: %v", err)
	}

	props := map[string]interface{}{}
	meta := map[string]interface{}{
		"network":         "https://www.googleapis.com/compute/v1/projects/p1/global/networks/prod-vpc",
		"ip_cidr_range":   "10.0.0.0/20",
		"gateway_address": "10.0.0.1",
	}
	source.extractGCPSubnetMetadata(props, meta)

	if props["vpc_id"] != "prod-vpc" {
		t.Errorf("vpc_id = %v, want prod-vpc", props["vpc_id"])
	}
	if props["ip_cidr_range"] != "10.0.0.0/20" {
		t.Errorf("ip_cidr_range = %v, want 10.0.0.0/20", props["ip_cidr_range"])
	}

	// Missing network must not set vpc_id.
	empty := map[string]interface{}{}
	source.extractGCPSubnetMetadata(empty, map[string]interface{}{})
	if _, ok := empty["vpc_id"]; ok {
		t.Errorf("vpc_id should be unset when network is absent")
	}
}

func TestCreateCDNEdges(t *testing.T) {
	source, err := NewGCPSource(GCPSourceConfig{}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatalf("Failed to create GCPSource: %v", err)
	}
	req := &core.SourceBuildRequest{TenantID: "t1", CloudAccountID: "acct-1"}

	backendPool := core.NewNode(core.NodeTypeBackendPool, "gcp:acct-1:global:BackendPool:web-bes:web-bes",
		map[string]interface{}{"name": "web-bes"}, req.TenantID, req.CloudAccountID, "gcp")
	cdnMatched := core.NewNode(core.NodeTypeCDN, "gcp:acct-1:global:CDN:web-bes",
		map[string]interface{}{"name": "web-bes", "origin_backend_service_name": "web-bes"}, req.TenantID, req.CloudAccountID, "gcp")
	cdnUnmatched := core.NewNode(core.NodeTypeCDN, "gcp:acct-1:global:CDN:orphan-bes",
		map[string]interface{}{"name": "orphan-bes", "origin_backend_service_name": "orphan-bes"}, req.TenantID, req.CloudAccountID, "gcp")

	lookup := newNodeLookup([]*core.DbNode{backendPool, cdnMatched, cdnUnmatched})
	edges := source.createCDNEdges(lookup, req)

	if len(edges) != 1 {
		t.Fatalf("expected 1 CDN edge, got %d", len(edges))
	}
	if edges[0].SourceNodeID != cdnMatched.ID || edges[0].DestinationNodeID != backendPool.ID {
		t.Errorf("CDN edge endpoints wrong: src=%s dst=%s", edges[0].SourceNodeID, edges[0].DestinationNodeID)
	}
	if edges[0].RelationshipType != core.RelationshipRoutesTo {
		t.Errorf("CDN edge relationship = %v, want ROUTES_TO", edges[0].RelationshipType)
	}
	if edges[0].Properties["connection_type"] != "cdn_backend" {
		t.Errorf("connection_type = %v, want cdn_backend", edges[0].Properties["connection_type"])
	}
}

func TestCreateNodeFromResourceProjectID(t *testing.T) {
	source, err := NewGCPSource(GCPSourceConfig{}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatalf("Failed to create GCPSource: %v", err)
	}
	req := &core.SourceBuildRequest{TenantID: "t1", CloudAccountID: "acct-uuid"}

	// Bare Name + populated account_number → project must come from account_number,
	// not from the resource's own name.
	bare := &CloudResourceRow{
		ID: "r1", Name: "default", Type: "subnet", ServiceName: "Networking",
		Region: "us-west3", ARN: "cbp-infra/regions/us-west3/subnetworks/default",
		AccountNumber: "cbp-infra", IsActive: true,
	}
	node := source.createNodeFromResource(bare, req)
	if node == nil {
		t.Fatal("expected a node, got nil")
	}
	if node.Properties["gcp_project_id"] != "cbp-infra" {
		t.Errorf("gcp_project_id = %v, want cbp-infra", node.Properties["gcp_project_id"])
	}

	// Empty account_number → fall back to parsing a path-style Name.
	fallback := &CloudResourceRow{
		ID: "r2", Name: "myproj/regions/us-west3/subnetworks/s", Type: "subnet",
		ServiceName: "Networking", Region: "us-west3", IsActive: true,
	}
	node2 := source.createNodeFromResource(fallback, req)
	if node2 == nil {
		t.Fatal("expected a node, got nil")
	}
	if node2.Properties["gcp_project_id"] != "myproj" {
		t.Errorf("fallback gcp_project_id = %v, want myproj", node2.Properties["gcp_project_id"])
	}
}

func TestCreatePubSubSubscriptionEdges(t *testing.T) {
	source, err := NewGCPSource(GCPSourceConfig{}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatalf("Failed to create GCPSource: %v", err)
	}
	req := &core.SourceBuildRequest{TenantID: "t1", CloudAccountID: "acct-1"}

	cloudRun := core.NewNode(core.NodeTypeServerlessFunction, "gcp:acct-1:us-central1:ServerlessFunction:cdc-events:cdc-events",
		map[string]interface{}{"name": "cdc-events", "dns_name": "cdc-events-abc-uc.a.run.app"},
		req.TenantID, req.CloudAccountID, "gcp")
	topic := core.NewNode(core.NodeTypeTopic, "gcp:acct-1:global:Topic:evt-topic:evt-topic",
		map[string]interface{}{"name": "evt-topic"}, req.TenantID, req.CloudAccountID, "gcp")
	lookup := newNodeLookup([]*core.DbNode{cloudRun, topic})

	subMeta := func(topic, push string) json.RawMessage {
		b, _ := json.Marshal(map[string]interface{}{"topic": topic, "push_endpoint": push})
		return b
	}
	resources := []CloudResourceRow{
		// push → Cloud Run: should link
		{Name: "evt-sub", Type: "pubsub.googleapis.com/Subscription",
			Meta: subMeta("evt-topic", "https://cdc-events-abc-uc.a.run.app?__GCP_CloudEventsMode=CE_PUBSUB_BINDING")},
		// push → external webhook: no in-graph consumer, skip
		{Name: "ext-sub", Type: "pubsub.googleapis.com/Subscription",
			Meta: subMeta("evt-topic", "https://api.example.com/webhook")},
		// pull subscription (no push): skip
		{Name: "pull-sub", Type: "pubsub.googleapis.com/Subscription",
			Meta: subMeta("evt-topic", "")},
		// not a subscription: ignore
		{Name: "evt-topic", Type: "pubsub.googleapis.com/Topic", Meta: json.RawMessage(`{}`)},
	}

	edges := source.createPubSubSubscriptionEdges(resources, lookup, req)
	if len(edges) != 1 {
		t.Fatalf("expected 1 subscription edge, got %d", len(edges))
	}
	e := edges[0]
	if e.SourceNodeID != cloudRun.ID || e.DestinationNodeID != topic.ID {
		t.Errorf("edge endpoints wrong: src=%s dst=%s", e.SourceNodeID, e.DestinationNodeID)
	}
	if e.RelationshipType != core.RelationshipSubscribesTo {
		t.Errorf("relationship = %v, want SUBSCRIBES_TO", e.RelationshipType)
	}
	if e.Properties["connection_type"] != "pubsub_push_subscription" {
		t.Errorf("connection_type = %v", e.Properties["connection_type"])
	}
}

func TestEnrichExistingURLMapNode(t *testing.T) {
	tenant, acct := "t1", "acct-1"
	// Resource-table url-map node (now typed RouteTable) with no routing target yet.
	existing := core.NewNode(core.NodeTypeRouteTable, "gcp:acct-1:global:RouteTable:lb-map:lb-map",
		map[string]interface{}{"name": "lb-map", "type": "url-map"}, tenant, acct, "gcp")
	lookup := newNodeLookup([]*core.DbNode{existing})

	urlMap := &GCPURLMap{
		Name:           "lb-map",
		DefaultService: "https://www.googleapis.com/compute/v1/projects/p/global/backendServices/web-bes",
		SelfLink:       "https://www.googleapis.com/compute/v1/projects/p/global/urlMaps/lb-map",
	}

	if !enrichExistingURLMapNode(lookup, "lb-map", urlMap) {
		t.Fatal("expected enrichExistingURLMapNode to find and enrich the node")
	}
	if existing.Properties["default_service"] != "web-bes" {
		t.Errorf("default_service = %v, want web-bes", existing.Properties["default_service"])
	}
	if existing.Properties["default_service_url"] != urlMap.DefaultService {
		t.Errorf("default_service_url not stamped")
	}

	// No matching node → returns false (caller creates a fresh node).
	if enrichExistingURLMapNode(lookup, "other", urlMap) {
		t.Error("expected false when no url-map node matches")
	}
}
