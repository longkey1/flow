package cmd

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseInputFlags(t *testing.T) {
	tests := []struct {
		name    string
		raw     []string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "empty",
			raw:  nil,
			want: map[string]string{},
		},
		{
			name: "single input",
			raw:  []string{"env=prod"},
			want: map[string]string{"env": "prod"},
		},
		{
			name: "multiple inputs",
			raw:  []string{"env=prod", "region=jp"},
			want: map[string]string{"env": "prod", "region": "jp"},
		},
		{
			name: "value contains equals",
			raw:  []string{"query=a=b"},
			want: map[string]string{"query": "a=b"},
		},
		{
			name: "empty value",
			raw:  []string{"env="},
			want: map[string]string{"env": ""},
		},
		{
			name:    "missing equals",
			raw:     []string{"env"},
			wantErr: true,
		},
		{
			name:    "leading equals",
			raw:     []string{"=value"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInputFlags(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseInputFlags(%v): expected error, got %v", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseInputFlags(%v): unexpected error: %v", tt.raw, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseInputFlags(%v) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDirsDefaultRoot(t *testing.T) {
	t.Setenv("FLOW_ROOT", "")

	base := "/base"
	if got, want := workflowsDir(base), filepath.Join(base, ".flow", "workflows"); got != want {
		t.Errorf("workflowsDir(%q) = %q, want %q", base, got, want)
	}
	if got, want := actionsDir(base), filepath.Join(base, ".flow", "actions"); got != want {
		t.Errorf("actionsDir(%q) = %q, want %q", base, got, want)
	}
	if got, want := logsDir(base), filepath.Join(base, ".flow", "logs"); got != want {
		t.Errorf("logsDir(%q) = %q, want %q", base, got, want)
	}
}

func TestDirsCustomRoot(t *testing.T) {
	t.Setenv("FLOW_ROOT", "custom-root")

	base := "/base"
	if got, want := workflowsDir(base), filepath.Join(base, "custom-root", "workflows"); got != want {
		t.Errorf("workflowsDir(%q) = %q, want %q", base, got, want)
	}
	if got, want := actionsDir(base), filepath.Join(base, "custom-root", "actions"); got != want {
		t.Errorf("actionsDir(%q) = %q, want %q", base, got, want)
	}
	if got, want := logsDir(base), filepath.Join(base, "custom-root", "logs"); got != want {
		t.Errorf("logsDir(%q) = %q, want %q", base, got, want)
	}
}
