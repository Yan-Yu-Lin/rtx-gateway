package usage

import "encoding/json"

type Extracted struct {
	Model            *string
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	UsagePresent     bool
}

func ExtractFromJSON(body []byte) Extracted {
	var payload struct {
		Model *string `json:"model"`
		Usage *struct {
			PromptTokens     *int `json:"prompt_tokens"`
			CompletionTokens *int `json:"completion_tokens"`
			TotalTokens      *int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return Extracted{}
	}

	result := Extracted{Model: payload.Model}
	if payload.Usage != nil {
		result.UsagePresent = true
		result.PromptTokens = payload.Usage.PromptTokens
		result.CompletionTokens = payload.Usage.CompletionTokens
		result.TotalTokens = payload.Usage.TotalTokens
	}
	return result
}

type Capture struct {
	Model            *string
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	Streaming        bool
	UsageMissing     bool
}

func (capture *Capture) Apply(extracted Extracted) {
	if extracted.Model != nil {
		capture.Model = extracted.Model
	}
	if !extracted.UsagePresent {
		return
	}
	capture.UsageMissing = false
	capture.PromptTokens = extracted.PromptTokens
	capture.CompletionTokens = extracted.CompletionTokens
	capture.TotalTokens = extracted.TotalTokens
}
