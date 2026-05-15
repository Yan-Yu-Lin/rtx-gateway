package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/usage"
)

func captureUsageFromResponse(response *http.Response, capture *usage.Capture) error {
	if isEventStream(response.Header.Get("Content-Type")) {
		capture.Streaming = true
		capture.UsageMissing = response.StatusCode >= 200 && response.StatusCode < 400
		response.Body = newSSEUsageBody(response.Body, capture)
		return nil
	}

	body, err := io.ReadAll(response.Body)
	if closeErr := response.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}

	extracted := usage.ExtractFromJSON(body)
	capture.Apply(extracted)
	if response.StatusCode >= 200 && response.StatusCode < 400 && !extracted.UsagePresent {
		capture.UsageMissing = true
	}

	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	response.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}

func isEventStream(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}
