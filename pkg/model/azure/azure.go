package azure

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
	APIKey     string
	BaseURL    string
	APIVersion string
	Deployment string
}

type Provider struct {
	config Config
	client *http.Client
}

func NewProvider(config Config) *Provider {
	if config.APIVersion == "" {
		config.APIVersion = "2024-02-15-preview"
	}
	return &Provider{
		config: config,
		client: &http.Client{},
	}
}

func (p *Provider) buildURL() string {
	baseURL := strings.TrimSuffix(p.config.BaseURL, "/")
	return fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		baseURL, p.config.Deployment, p.config.APIVersion)
}

func (p *Provider) Send(ctx context.Context, req *model.Request) (*model.Response, error) {
	req.Stream = false
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.buildURL(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", p.config.APIKey)

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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.buildURL(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", p.config.APIKey)
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
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				ch <- &model.ResponseEvent{FinishReason: model.FinishReasonStop}
			}
			return
		}

		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}

		line = bytes.TrimPrefix(line, []byte("data: "))
		if bytes.Equal(line, []byte("[DONE]")) {
			ch <- &model.ResponseEvent{FinishReason: model.FinishReasonStop}
			return
		}

		var chunk model.StreamChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				ch <- &model.ResponseEvent{
					Delta: choice.Delta.Content,
				}
			}
			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					ch <- &model.ResponseEvent{
						ToolCall: &tc,
					}
				}
			}
			if choice.FinishReason != "" {
				ch <- &model.ResponseEvent{
					FinishReason: model.FinishReason(choice.FinishReason),
				}
			}
		}
	}
}
