package cmd

import "testing"

func TestOverwriteFlagValue(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      bool
		wantError bool
	}{
		{name: "true", value: "true", want: true},
		{name: "false", value: "false", want: false},
		{name: "uppercase rejected", value: "TRUE", wantError: true},
		{name: "numeric rejected", value: "1", wantError: true},
		{name: "empty rejected", value: "", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := false
			err := overwriteFlagValue{value: &got}.Set(tt.value)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("value = %v, want %v", got, tt.want)
			}
		})
	}
}
