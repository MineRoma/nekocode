package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/m1neroma/neko/internal/config"
	"github.com/m1neroma/neko/internal/core"
)

type Request struct {
	Model     string
	Messages  []core.Message
	Tools     []core.ToolDefinition
	MaxTokens int
	OnText    func(string)
}

type Response struct {
	Message core.Message
	Usage   core.Usage
}

type Client interface {
	Chat(context.Context, Request) (Response, error)
	ListModels(context.Context) ([]config.Model, error)
}

type client struct {
	cfg  config.Provider
	http *http.Client
}

func New(cfg config.Provider) (Client, error) {
	c := &client{cfg: cfg, http: &http.Client{Timeout: 15 * time.Minute}}
	switch strings.ToLower(cfg.Compatibility) {
	case "openai":
		return &openAI{client: c}, nil
	case "anthropic":
		return &anthropic{client: c}, nil
	default:
		return nil, fmt.Errorf("unsupported provider compatibility %q", cfg.Compatibility)
	}
}

func (c *client) apiKey() (string, error) {
	if c.cfg.APIKey != "" {
		return c.cfg.APIKey, nil
	}
	// Older Neko builds accepted a pasted key in the environment-variable field.
	// Keep those configurations working while the new wizard stores pasted keys explicitly.
	if strings.HasPrefix(c.cfg.APIKeyEnv, "sk-") {
		return c.cfg.APIKeyEnv, nil
	}
	if c.cfg.APIKeyEnv == "" {
		return "", errors.New("provider API key is not configured")
	}
	key := os.Getenv(c.cfg.APIKeyEnv)
	if key == "" {
		return "", fmt.Errorf("environment variable %s is empty", c.cfg.APIKeyEnv)
	}
	return key, nil
}

func endpoint(base, suffix string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(suffix, "/")
}

func decodeAPIError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	return fmt.Errorf("provider returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
}

type openAI struct{ *client }

func (o *openAI) Chat(ctx context.Context, req Request) (Response, error) {
	key, err := o.apiKey()
	if err != nil {
		return Response{}, err
	}
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		item := map[string]any{"role": msg.Role}
		if msg.Content != "" || len(msg.ToolCalls) == 0 {
			item["content"] = msg.Content
		}
		if msg.ToolCallID != "" {
			item["tool_call_id"] = msg.ToolCallID
		}
		if msg.Name != "" {
			item["name"] = msg.Name
		}
		if len(msg.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				calls = append(calls, map[string]any{
					"id":       call.ID,
					"type":     "function",
					"function": map[string]any{"name": call.Name, "arguments": string(call.Arguments)},
				})
			}
			item["tool_calls"] = calls
		}
		messages = append(messages, item)
	}
	payload := map[string]any{
		"model": req.Model, "messages": messages, "stream": true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema,
				},
			})
		}
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(o.cfg.BaseURL, "chat/completions"), bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := o.http.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, decodeAPIError(resp)
	}
	type accumulator struct {
		id   string
		name string
		args strings.Builder
	}
	calls := map[int]*accumulator{}
	var text strings.Builder
	var usage core.Usage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return Response{}, fmt.Errorf("decode OpenAI stream: %w", err)
		}
		usage.InputTokens = chunk.Usage.PromptTokens
		usage.OutputTokens = chunk.Usage.CompletionTokens
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				text.WriteString(choice.Delta.Content)
				if req.OnText != nil {
					req.OnText(choice.Delta.Content)
				}
			}
			for _, delta := range choice.Delta.ToolCalls {
				acc := calls[delta.Index]
				if acc == nil {
					acc = &accumulator{}
					calls[delta.Index] = acc
				}
				if delta.ID != "" {
					acc.id = delta.ID
				}
				if delta.Function.Name != "" {
					acc.name = delta.Function.Name
				}
				acc.args.WriteString(delta.Function.Arguments)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Response{}, err
	}
	indices := make([]int, 0, len(calls))
	for index := range calls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	toolCalls := make([]core.ToolCall, 0, len(indices))
	for _, index := range indices {
		acc := calls[index]
		args := json.RawMessage(acc.args.String())
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		toolCalls = append(toolCalls, core.ToolCall{ID: acc.id, Name: acc.name, Arguments: args})
	}
	if usage.InputTokens == 0 {
		usage.InputTokens = estimateMessages(req.Messages)
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = max(1, len(text.String())/4)
	}
	return Response{Message: core.Message{Role: "assistant", Content: text.String(), ToolCalls: toolCalls}, Usage: usage}, nil
}

func (o *openAI) ListModels(ctx context.Context) ([]config.Model, error) {
	key, err := o.apiKey()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(o.cfg.BaseURL, "models"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := o.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeAPIError(resp)
	}
	var result struct {
		Data []struct {
			ID            string `json:"id"`
			ContextWindow int    `json:"context_window"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8*1024*1024)).Decode(&result); err != nil {
		return nil, err
	}
	models := make([]config.Model, 0, len(result.Data))
	for _, model := range result.Data {
		if model.ID != "" {
			models = append(models, config.Model{ID: model.ID, ContextWindow: model.ContextWindow})
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

type anthropic struct{ *client }

func (a *anthropic) Chat(ctx context.Context, req Request) (Response, error) {
	key, err := a.apiKey()
	if err != nil {
		return Response{}, err
	}
	var systems []string
	messages := make([]map[string]any, 0, len(req.Messages))
	appendMessage := func(role string, blocks []map[string]any) {
		if len(messages) > 0 && messages[len(messages)-1]["role"] == role {
			existing := messages[len(messages)-1]["content"].([]map[string]any)
			messages[len(messages)-1]["content"] = append(existing, blocks...)
			return
		}
		messages = append(messages, map[string]any{"role": role, "content": blocks})
	}
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systems = append(systems, msg.Content)
			continue
		}
		if msg.Role == "tool" {
			appendMessage("user", []map[string]any{{"type": "tool_result", "tool_use_id": msg.ToolCallID, "content": msg.Content}})
			continue
		}
		role := msg.Role
		if role != "assistant" {
			role = "user"
		}
		var blocks []map[string]any
		if msg.Content != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": msg.Content})
		}
		for _, call := range msg.ToolCalls {
			var input any
			if err := json.Unmarshal(call.Arguments, &input); err != nil {
				input = map[string]any{}
			}
			blocks = append(blocks, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": input})
		}
		if len(blocks) > 0 {
			appendMessage(role, blocks)
		}
	}
	payload := map[string]any{
		"model": req.Model, "messages": messages, "stream": true, "max_tokens": max(req.MaxTokens, 4096),
	}
	if len(systems) > 0 {
		payload["system"] = strings.Join(systems, "\n\n")
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.InputSchema})
		}
		payload["tools"] = tools
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(a.cfg.BaseURL, "messages"), bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("x-api-key", key)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("accept", "text/event-stream")
	resp, err := a.http.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, decodeAPIError(resp)
	}
	type accumulator struct {
		id   string
		name string
		args strings.Builder
	}
	calls := map[int]*accumulator{}
	var text strings.Builder
	var usage core.Usage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var event struct {
			Type    string `json:"type"`
			Index   int    `json:"index"`
			Message struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return Response{}, fmt.Errorf("decode Anthropic stream: %w", err)
		}
		switch event.Type {
		case "message_start":
			usage.InputTokens = event.Message.Usage.InputTokens
		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				calls[event.Index] = &accumulator{id: event.ContentBlock.ID, name: event.ContentBlock.Name}
			}
		case "content_block_delta":
			if event.Delta.Type == "text_delta" {
				text.WriteString(event.Delta.Text)
				if req.OnText != nil {
					req.OnText(event.Delta.Text)
				}
			} else if event.Delta.Type == "input_json_delta" && calls[event.Index] != nil {
				calls[event.Index].args.WriteString(event.Delta.PartialJSON)
			}
		case "message_delta":
			usage.OutputTokens = event.Usage.OutputTokens
		case "error":
			return Response{}, errors.New(event.Error.Message)
		}
	}
	if err := scanner.Err(); err != nil {
		return Response{}, err
	}
	indices := make([]int, 0, len(calls))
	for index := range calls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	toolCalls := make([]core.ToolCall, 0, len(indices))
	for _, index := range indices {
		acc := calls[index]
		args := json.RawMessage(acc.args.String())
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		toolCalls = append(toolCalls, core.ToolCall{ID: acc.id, Name: acc.name, Arguments: args})
	}
	if usage.InputTokens == 0 {
		usage.InputTokens = estimateMessages(req.Messages)
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = max(1, len(text.String())/4)
	}
	return Response{Message: core.Message{Role: "assistant", Content: text.String(), ToolCalls: toolCalls}, Usage: usage}, nil
}

func (a *anthropic) ListModels(ctx context.Context) ([]config.Model, error) {
	key, err := a.apiKey()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(a.cfg.BaseURL, "models?limit=1000"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeAPIError(resp)
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8*1024*1024)).Decode(&result); err != nil {
		return nil, err
	}
	models := make([]config.Model, 0, len(result.Data))
	for _, model := range result.Data {
		if model.ID != "" {
			models = append(models, config.Model{ID: model.ID})
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func estimateMessages(messages []core.Message) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content)
		for _, call := range msg.ToolCalls {
			total += len(call.Name) + len(call.Arguments)
		}
	}
	return max(1, total/4)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
