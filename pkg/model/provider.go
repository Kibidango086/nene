package model

import (
	"context"
	"encoding/json"
	"io"
)

type FinishReason string

const (
	FinishReasonStop      FinishReason = "stop"
	FinishReasonToolCalls FinishReason = "tool_calls"
)

type ResponseEvent struct {
	Delta        string
	ToolCall     *ToolCall
	FinishReason FinishReason
}

type Provider interface {
	Send(ctx context.Context, req *Request) (*Response, error)
	SendStream(ctx context.Context, req *Request) (<-chan *ResponseEvent, error)
}

type ProviderFunc func(ctx context.Context, req *Request) (*Response, error)

func (f ProviderFunc) Send(ctx context.Context, req *Request) (*Response, error) {
	return f(ctx, req)
}

func (f ProviderFunc) SendStream(ctx context.Context, req *Request) (<-chan *ResponseEvent, error) {
	resp, err := f(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan *ResponseEvent, 1)
	go func() {
		defer close(ch)
		if len(resp.Choices) > 0 {
			choice := resp.Choices[0]
			ch <- &ResponseEvent{
				Delta:        choice.Message.Content,
				FinishReason: FinishReason(choice.FinishReason),
			}
			for _, tc := range choice.Message.ToolCalls {
				ch <- &ResponseEvent{
					ToolCall: &tc,
				}
			}
		}
	}()
	return ch, nil
}

func NewFunctionTool(name, description string, parameters json.RawMessage) Tool {
	return Tool{
		Type: "function",
		Function: FunctionTool{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}
}

type StreamReader interface {
	Recv() (*ResponseEvent, error)
	io.Closer
}
