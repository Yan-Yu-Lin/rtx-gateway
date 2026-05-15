package usage

import "testing"

func TestExtractFromJSONWithUsage(t *testing.T) {
	extracted := ExtractFromJSON([]byte(`{
		"model": "gemma-4",
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 20,
			"total_tokens": 30
		}
	}`))

	if extracted.Model == nil || *extracted.Model != "gemma-4" {
		t.Fatalf("model = %v, want gemma-4", extracted.Model)
	}
	if !extracted.UsagePresent {
		t.Fatal("UsagePresent = false, want true")
	}
	if extracted.PromptTokens == nil || *extracted.PromptTokens != 10 {
		t.Fatalf("prompt tokens = %v, want 10", extracted.PromptTokens)
	}
	if extracted.CompletionTokens == nil || *extracted.CompletionTokens != 20 {
		t.Fatalf("completion tokens = %v, want 20", extracted.CompletionTokens)
	}
	if extracted.TotalTokens == nil || *extracted.TotalTokens != 30 {
		t.Fatalf("total tokens = %v, want 30", extracted.TotalTokens)
	}
}

func TestExtractFromJSONMissingUsage(t *testing.T) {
	extracted := ExtractFromJSON([]byte(`{"model":"chandra","choices":[]}`))
	if extracted.Model == nil || *extracted.Model != "chandra" {
		t.Fatalf("model = %v, want chandra", extracted.Model)
	}
	if extracted.UsagePresent {
		t.Fatal("UsagePresent = true, want false")
	}
}

func TestExtractFromJSONInvalid(t *testing.T) {
	extracted := ExtractFromJSON([]byte(`not-json`))
	if extracted.Model != nil || extracted.UsagePresent {
		t.Fatalf("invalid JSON extracted unexpected data: %+v", extracted)
	}
}
