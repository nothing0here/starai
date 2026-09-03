package service

import "testing"

func TestParseWorkRetentionDays(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		fallback int
		want     int
	}{
		{name: "json number", value: float64(7), fallback: 30, want: 7},
		{name: "numeric string", value: "14", fallback: 30, want: 14},
		{name: "permanent", value: float64(0), fallback: 30, want: 0},
		{name: "invalid falls back", value: "invalid", fallback: 7, want: 7},
		{name: "negative is permanent", value: float64(-1), fallback: 7, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ParseWorkRetentionDays(test.value, test.fallback); got != test.want {
				t.Fatalf("ParseWorkRetentionDays(%v, %d) = %d, want %d", test.value, test.fallback, got, test.want)
			}
		})
	}
}
