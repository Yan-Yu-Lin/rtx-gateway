package proxy

import (
	"bytes"
	"io"
	"strings"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/usage"
)

type sseUsageBody struct {
	body    io.ReadCloser
	capture *usage.Capture
	buffer  []byte
}

func newSSEUsageBody(body io.ReadCloser, capture *usage.Capture) io.ReadCloser {
	return &sseUsageBody{body: body, capture: capture}
}

func (body *sseUsageBody) Read(target []byte) (int, error) {
	n, err := body.body.Read(target)
	if n > 0 {
		body.parse(target[:n])
	}
	if err == io.EOF && len(body.buffer) > 0 {
		body.processLine(body.buffer)
		body.buffer = nil
	}
	return n, err
}

func (body *sseUsageBody) Close() error {
	return body.body.Close()
}

func (body *sseUsageBody) parse(chunk []byte) {
	body.buffer = append(body.buffer, chunk...)
	for {
		index := bytes.IndexByte(body.buffer, '\n')
		if index < 0 {
			return
		}

		line := body.buffer[:index]
		body.processLine(line)
		body.buffer = body.buffer[index+1:]
	}
}

func (body *sseUsageBody) processLine(raw []byte) {
	line := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(line, "data:") {
		return
	}

	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "" || data == "[DONE]" {
		return
	}

	body.capture.Apply(usage.ExtractFromJSON([]byte(data)))
}
