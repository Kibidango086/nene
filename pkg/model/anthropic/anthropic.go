package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/nene-agent/nene/pkg/model"
)

type Config struct {
	APIKey  string
	BaseURL string
	Model   string
}

type Provider struct {
	config Config
	client *http.Client
}

func NewProvider(config Config) *Provider {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.anthropic.com/v1"
	}
	return &Provider{
		config: config,
		client: &http.Client{},
	}
}

type anthropicRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []anthropicMsg  `json:"messages"`
	System    string          `json:"system,omitempty"`
	Tools     []anthropicTool `json:"tools,omitempty"`
	Stream    bool            `json:"stream"`
}

type anthropicMsg struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Content      []anthropicContent `json:"content"`
	Model        string             `json:"model"`
	StopReason   string             `json:"stop_reason"`
	StopSequence string             `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicStreamEvent struct {
	Type         string             `json:"type"`
	Index        int                `json:"index"`
	Delta        *anthropicDelta    `json:"delta,omitempty"`
	ContentBlock *anthropicContent  `json:"content_block,omitempty"`
	Message      *anthropicResponse `json:"message,omitempty"`
}

type anthropicDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

func convertToAnthropicRequest(req *model.Request) *anthropicRequest {
	ar := &anthropicRequest{
		Model:     req.Model,
		MaxTokens: 4096,
		Messages:  make([]anthropicMsg, 0),
		Stream:    req.Stream,
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			ar.System = msg.Content
		case "user":
			ar.Messages = append(ar.Messages, anthropicMsg{
				Role:    "user",
				Content: []anthropicContent{{Type: "text", Text: msg.Content}},
			})
		case "assistant":
			ar.Messages = append(ar.Messages, anthropicMsg{
				Role:    "assistant",
				Content: []anthropicContent{{Type: "text", Text: msg.Content}},
			})
		case "tool":
			ar.Messages = append(ar.Messages, anthropicMsg{
				Role: "user",
				Content: []anthropicContent{{
					Type: "tool_result",
					Text: msg.Content,
				}},
			})
		}
	}

	for _, tool := range req.Tools {
		ar.Tools = append(ar.Tools, anthropicTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}

	return ar
}

func (p *Provider) Send(ctx context.Context, req *model.Request) (*model.Response, error) {
	ar := convertToAnthropicRequest(req)
	ar.Stream = false

	body, err := json.Marshal(ar)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var aResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&aResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return convertToModelResponse(&aResp), nil
}

func convertToModelResponse(aResp *anthropicResponse) *model.Response {
	resp := &model.Response{
		ID:      aResp.ID,
		Model:   aResp.Model,
		Choices: make([]model.Choice, 1),
		Usage: model.Usage{
			PromptTokens:     aResp.Usage.InputTokens,
			CompletionTokens: aResp.Usage.OutputTokens,
			TotalTokens:      aResp.Usage.InputTokens + aResp.Usage.OutputTokens,
		},
	}

	var content string
	for _, c := range aResp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	finishReason := "stop"
	if aResp.StopReason == "tool_use" {
		finishReason = "tool_calls"
	}

	resp.Choices[0] = model.Choice{
		Message: model.Message{
			Role:    "assistant",
			Content: content,
		},
		FinishReason: finishReason,
	}

	return resp
}

func (p *Provider) SendStream(ctx context.Context, req *model.Request) (<-chan *model.ResponseEvent, error) {
	ar := convertToAnthropicRequest(req)
	ar.Stream = true

	body, err := json.Marshal(ar)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	ch := make(chan *model.ResponseEvent, 100)
	go p.readStream(resp.Body, ch)

	return ch, nil
}

func (p *Provider) readStream(body io.ReadCloser, ch chan<- *model.ResponseEvent) {
	defer body.Close()
	defer close(ch)

	reader := bufio.NewReader(body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				ch <- &model.ResponseEvent{FinishReason: model.FinishReasonStop}
			}
			return
		}

		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		line = strings.TrimPrefix(line, "data: ")
		if line == "" {
			continue
		}

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta != nil && event.Delta.Text != "" {
				ch <- &model.ResponseEvent{
					Delta: event.Delta.Text,
				}
			}
		case "content_block_stop":
			// Content block finished
		case "message_stop":
			ch <- &model.ResponseEvent{FinishReason: model.FinishReasonStop}
			return
		case "message_delta":
			if event.Delta != nil && event.Delta.StopReason == "tool_use" {
				ch <- &model.ResponseEvent{FinishReason: model.FinishReasonToolCalls}
			}
		}
	}
}
