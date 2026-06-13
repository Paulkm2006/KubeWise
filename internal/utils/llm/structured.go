package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CompleteJSON calls the model and decodes the response into dest. It uses
// provider JSON mode when possible and falls back to strict prompt + extraction.
func (c *Client) CompleteJSON(ctx context.Context, req CompletionRequest, schema map[string]any, dest any) (*CompletionResponse, error) {
	return CompleteJSON(ctx, c, req, schema, dest)
}

func CompleteJSON(ctx context.Context, client ClientPort, req CompletionRequest, schema map[string]any, dest any) (*CompletionResponse, error) {
	req.Messages = withJSONInstruction(req.Messages, schema, "")
	req.ResponseFormat = &ResponseFormat{
		Type:   ResponseFormatJSONObject,
		Name:   "structured_response",
		Schema: schema,
		Strict: true,
	}

	resp, err := CompleteWithRetry(ctx, client, req, DefaultRetryPolicy())
	if err != nil {
		return nil, err
	}
	if err := decodeJSONContent(resp.Message.Content, dest); err == nil {
		return resp, nil
	} else {
		resp.ParseError = err.Error()
	}

	repairReq := req
	repairReq.Messages = withJSONInstruction(req.Messages, schema, resp.ParseError)
	repairResp, repairErr := CompleteWithRetry(ctx, client, repairReq, DefaultRetryPolicy())
	if repairErr != nil {
		return resp, repairErr
	}
	if err := decodeJSONContent(repairResp.Message.Content, dest); err != nil {
		repairResp.ParseError = err.Error()
		return repairResp, fmt.Errorf("decode structured JSON: %w", err)
	}
	return repairResp, nil
}

func withJSONInstruction(messages []Message, schema map[string]any, repair string) []Message {
	out := make([]Message, 0, len(messages)+1)
	out = append(out, messages...)
	schemaText := "{}"
	if schema != nil {
		if bs, err := json.Marshal(schema); err == nil {
			schemaText = string(bs)
		}
	}
	content := "Return only valid JSON matching this schema. Do not wrap it in markdown fences.\nSchema:\n" + schemaText
	if repair != "" {
		content += "\nPrevious response failed to parse/validate: " + repair + "\nReturn corrected JSON only."
	}
	out = append(out, Message{Role: "system", Content: content})
	return out
}

func decodeJSONContent(content string, dest any) error {
	raw, err := extractJSONObject(content)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return err
	}
	return nil
}

func extractJSONObject(content string) (string, error) {
	s := strings.TrimSpace(content)
	if s == "" {
		return "", fmt.Errorf("empty model response")
	}
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 3 {
			lines = lines[1 : len(lines)-1]
			s = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	if json.Valid([]byte(s)) {
		return s, nil
	}
	startObj := strings.Index(s, "{")
	startArr := strings.Index(s, "[")
	start := startObj
	if start == -1 || (startArr >= 0 && startArr < start) {
		start = startArr
	}
	if start < 0 {
		return "", fmt.Errorf("no JSON object or array found")
	}
	for end := len(s); end > start; end-- {
		candidate := strings.TrimSpace(s[start:end])
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("response does not contain valid JSON")
}
