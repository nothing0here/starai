package service

import "testing"

func TestValidateComposeTaskInput(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateComposeTaskInput
		wantErr bool
	}{
		{
			name: "auto video and audio",
			input: CreateComposeTaskInput{
				Sources: []map[string]interface{}{{"kind": "video"}, {"kind": "audio"}},
				Mode:    "auto", OutputSize: "keep",
			},
		},
		{
			name: "concat same kind",
			input: CreateComposeTaskInput{
				Sources: []map[string]interface{}{{"kind": "video"}, {"kind": "video"}},
				Mode:    "concat", OutputSize: "1080x1920",
			},
		},
		{
			name: "concat mixed kinds",
			input: CreateComposeTaskInput{
				Sources: []map[string]interface{}{{"kind": "video"}, {"kind": "audio"}},
				Mode:    "concat", OutputSize: "keep",
			},
			wantErr: true,
		},
		{
			name: "mux requires exactly one audio",
			input: CreateComposeTaskInput{
				Sources: []map[string]interface{}{{"kind": "video"}, {"kind": "audio"}, {"kind": "audio"}},
				Mode:    "mux", OutputSize: "keep",
			},
			wantErr: true,
		},
		{
			name: "auto refuses ignored image",
			input: CreateComposeTaskInput{
				Sources: []map[string]interface{}{{"kind": "image"}, {"kind": "video"}},
				Mode:    "auto", OutputSize: "keep",
			},
			wantErr: true,
		},
		{
			name: "invalid output size",
			input: CreateComposeTaskInput{
				Sources: []map[string]interface{}{{"kind": "audio"}},
				Mode:    "auto", OutputSize: "999x999",
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateComposeTaskInput(&test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateComposeTaskInput() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateComposeTaskInputAppliesDefaults(t *testing.T) {
	input := CreateComposeTaskInput{
		Sources: []map[string]interface{}{{"kind": "audio"}},
	}
	if err := validateComposeTaskInput(&input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.Mode != "auto" || input.OutputSize != "keep" {
		t.Fatalf("defaults = mode %q, size %q", input.Mode, input.OutputSize)
	}
}
