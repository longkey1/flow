package cmd

import "testing"

func TestJoin(t *testing.T) {
	tests := []struct {
		name string
		ss   []string
		want string
	}{
		{name: "empty", ss: nil, want: ""},
		{name: "single", ss: []string{"a"}, want: "a"},
		{name: "two", ss: []string{"a", "b"}, want: "a, b"},
		{name: "three", ss: []string{"build", "test", "deploy"}, want: "build, test, deploy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := join(tt.ss); got != tt.want {
				t.Errorf("join(%v) = %q, want %q", tt.ss, got, tt.want)
			}
		})
	}
}
