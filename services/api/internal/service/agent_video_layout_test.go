package service

import "testing"

func TestAgentVideoLayoutCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		prop                   map[string]interface{}
		target, count, seconds int
		invalid                bool
	}{
		{"fixed8", map[string]interface{}{"enum": []interface{}{8}}, 15, 2, 8, false},
		{"stringSeconds", map[string]interface{}{"enum": []interface{}{"4s", "8s"}}, 22, 3, 8, false},
		{"range", map[string]interface{}{"minimum": 4, "maximum": 15}, 22, 2, 11, false},
		{"customRange", map[string]interface{}{"enum": []interface{}{5, 10}, "x-allow-custom": true, "minimum": 2, "maximum": 20}, 36, 2, 18, false},
		{"step", map[string]interface{}{"minimum": 4, "maximum": 16, "multipleOf": 4}, 22, 2, 12, false},
		{"autoIsNotLimit", map[string]interface{}{"enum": []interface{}{-1, 5, 15}}, 16, 2, 15, false},
		{"defaultNotCapability", map[string]interface{}{"default": 8}, 15, 0, 0, true},
		{"autoOnlyUnknown", map[string]interface{}{"enum": []interface{}{-1}}, 15, 0, 0, true},
		{"zero", map[string]interface{}{"enum": []interface{}{8}}, 0, 0, 0, true},
		{"tooLong", map[string]interface{}{"enum": []interface{}{8}}, 601, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &ModelFull{ModelDTO: ModelDTO{InputSchema: map[string]interface{}{"properties": map[string]interface{}{"duration": tc.prop}}, DefaultParams: map[string]interface{}{"duration": 8}}}
			count, seconds, err := AgentVideoLayout(model, tc.target)
			if (err != nil) != tc.invalid || count != tc.count || seconds != tc.seconds {
				t.Fatalf("got %d x %d, %v; want %d x %d invalid=%v", count, seconds, err, tc.count, tc.seconds, tc.invalid)
			}
		})
	}
}
