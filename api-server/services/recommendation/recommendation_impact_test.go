package recommendation

import "testing"

func TestExtractK8sRecRef(t *testing.T) {
	tests := []struct {
		name   string
		rec    map[string]any
		wantNS string
		wantWL string
		wantOK bool
	}{
		{
			name:   "flat controller_name",
			rec:    map[string]any{"namespace": "shop", "controller_name": "checkout"},
			wantNS: "shop", wantWL: "checkout", wantOK: true,
		},
		{
			name:   "camelCase controllerName",
			rec:    map[string]any{"namespace": "shop", "controllerName": "orders"},
			wantNS: "shop", wantWL: "orders", wantOK: true,
		},
		{
			name:   "nested workload object",
			rec:    map[string]any{"workload": map[string]any{"name": "ingester", "namespace": "data", "kind": "Deployment"}},
			wantNS: "data", wantWL: "ingester", wantOK: true,
		},
		{
			name:   "bare name fallback with whitespace trimmed",
			rec:    map[string]any{"namespace": " prod ", "name": " api "},
			wantNS: "prod", wantWL: "api", wantOK: true,
		},
		{
			name:   "missing namespace is unresolved",
			rec:    map[string]any{"controller_name": "checkout"},
			wantOK: false,
		},
		{
			name:   "missing workload name is unresolved",
			rec:    map[string]any{"namespace": "shop"},
			wantOK: false,
		},
		{
			name:   "empty map is unresolved",
			rec:    map[string]any{},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractK8sRecRef(tt.rec)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (got %+v)", ok, tt.wantOK, got)
			}
			if !tt.wantOK {
				return
			}
			if got.Namespace != tt.wantNS || got.Workload != tt.wantWL {
				t.Errorf("ref = {ns=%q wl=%q}, want {ns=%q wl=%q}", got.Namespace, got.Workload, tt.wantNS, tt.wantWL)
			}
		})
	}
}

func TestFirstNonEmptyString(t *testing.T) {
	m := map[string]any{"a": "", "b": "  ", "c": "val", "d": 5}
	if got := firstNonEmptyString(m, "a", "b", "c"); got != "val" {
		t.Errorf("got %q, want \"val\" (should skip empty/blank)", got)
	}
	if got := firstNonEmptyString(m, "d"); got != "" {
		t.Errorf("got %q, want empty (non-string value must be ignored)", got)
	}
	if got := firstNonEmptyString(m, "missing"); got != "" {
		t.Errorf("got %q, want empty (missing key)", got)
	}
}
