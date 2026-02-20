package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
		config.BaseURL = "https://api.openai.com/v1"
	}
	return &Provider{
		config: config,
		client: &http.Client{},
	}
}

func (p *Provider) Send(ctx context.Context, req *model.Request) (*model.Response, error) {
	req.Stream = false
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var response model.Response
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &response, nil
}

func (p *Provider) SendStream(ctx context.Context, req *model.Request) (<-chan *model.ResponseEvent, error) {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
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

type streamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (p *Provider) readStream(body io.ReadCloser, ch chan<- *model.ResponseEvent) {
	defer body.Close()
	defer close(ch)

	toolCallsMap := make(map[int]*model.ToolCall)

	reader := bufio.NewReader(body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
			}
			break
		}

		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}

		line = bytes.TrimPrefix(line, []byte("data: "))
		if bytes.Equal(line, []byte("[DONE]")) {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				ch <- &model.ResponseEvent{
					Delta: choice.Delta.Content,
				}
			}

			for _, tc := range choice.Delta.ToolCalls {
				idx := tc.Index
				if tc.ID != "" {
					toolCallsMap[idx] = &model.ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: model.FunctionCall{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				} else if existing, ok := toolCallsMap[idx]; ok {
					existing.Function.Arguments += tc.Function.Arguments
				}
			}

			if choice.FinishReason != "" {
				if choice.FinishReason == "tool_calls" {
					for _, tc := range toolCallsMap {
						ch <- &model.ResponseEvent{
							ToolCall: tc,
						}
					}
				}
				ch <- &model.ResponseEvent{
					FinishReason: model.FinishReason(choice.FinishReason),
				}
			}
		}
	}
}
