package ratio_setting

import "testing"

func TestClaude5DefaultRatios(t *testing.T) {
	InitRatioSettings()

	tests := []struct {
		model      string
		inputRatio float64
	}{
		{model: "claude-sonnet-5", inputRatio: 1.5},
		{model: "claude-opus-5", inputRatio: 2.5},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			ratio, found, _ := GetModelRatio(test.model)
			if !found || ratio != test.inputRatio {
				t.Fatalf("GetModelRatio(%q) = (%v, %v), want (%v, true)", test.model, ratio, found, test.inputRatio)
			}
			if ratio := GetCompletionRatio(test.model); ratio != 5 {
				t.Fatalf("GetCompletionRatio(%q) = %v, want 5", test.model, ratio)
			}
			if ratio, found := GetCacheRatio(test.model); !found || ratio != 0.1 {
				t.Fatalf("GetCacheRatio(%q) = (%v, %v), want (0.1, true)", test.model, ratio, found)
			}
			if ratio, found := GetCreateCacheRatio(test.model); !found || ratio != 1.25 {
				t.Fatalf("GetCreateCacheRatio(%q) = (%v, %v), want (1.25, true)", test.model, ratio, found)
			}
		})
	}
}
