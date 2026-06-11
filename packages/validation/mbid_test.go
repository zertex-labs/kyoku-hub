package validation

import "testing"

func TestIsMBID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"valid mbid", "5b11f4ce-a62d-471e-81fc-a69a8278c7da", true},
		{"empty", "", false},
		{"uppercase rejected", "5B11F4CE-A62D-471E-81FC-A69A8278C7DA", false},
		{"missing dashes", "5b11f4cea62d471e81fca69a8278c7da", false},
		{"too short", "5b11f4ce-a62d-471e-81fc", false},
		{"trailing garbage", "5b11f4ce-a62d-471e-81fc-a69a8278c7da'--", false},
		{"non-hex chars", "5b11f4ce-a62d-471e-81fc-a69a8278c7dz", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsMBID(tt.in); got != tt.want {
				t.Errorf("IsMBID(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
