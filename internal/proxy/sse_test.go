package proxy

import (
	"io"
	"strings"
	"testing"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/usage"
)

func TestSSEUsageBodyPreservesBytesAndExtractsUsage(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"model":"gemma-4","choices":[{"delta":{"content":"hi"}}]}`,
		"",
		`data: {"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	capture := usage.Capture{Streaming: true, UsageMissing: true}
	reader := newSSEUsageBody(io.NopCloser(strings.NewReader(stream)), &capture)
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if string(out) != stream {
		t.Fatalf("stream was modified\n got: %q\nwant: %q", string(out), stream)
	}
	if capture.Model == nil || *capture.Model != "gemma-4" {
		t.Fatalf("model = %v, want gemma-4", capture.Model)
	}
	if capture.UsageMissing {
		t.Fatal("UsageMissing = true, want false")
	}
	if capture.PromptTokens == nil || *capture.PromptTokens != 3 {
		t.Fatalf("prompt tokens = %v, want 3", capture.PromptTokens)
	}
	if capture.CompletionTokens == nil || *capture.CompletionTokens != 5 {
		t.Fatalf("completion tokens = %v, want 5", capture.CompletionTokens)
	}
	if capture.TotalTokens == nil || *capture.TotalTokens != 8 {
		t.Fatalf("total tokens = %v, want 8", capture.TotalTokens)
	}
}

func TestSSEUsageBodyKeepsUsageMissingWithoutUsageChunk(t *testing.T) {
	stream := "data: {\"model\":\"chandra\",\"choices\":[{}]}\n\ndata: [DONE]\n\n"
	capture := usage.Capture{Streaming: true, UsageMissing: true}

	if _, err := io.ReadAll(newSSEUsageBody(io.NopCloser(strings.NewReader(stream)), &capture)); err != nil {
		t.Fatal(err)
	}

	if capture.Model == nil || *capture.Model != "chandra" {
		t.Fatalf("model = %v, want chandra", capture.Model)
	}
	if !capture.UsageMissing {
		t.Fatal("UsageMissing = false, want true")
	}
}
