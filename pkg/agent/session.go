package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/nene-agent/nene/pkg/bus"
	"github.com/nene-agent/nene/pkg/model"
	"github.com/nene-agent/nene/pkg/tool"
)

type Session struct {
	modelName    string
	provider     model.Provider
	toolMgr      *tool.Manager
	systemPrompt string
	bus          *bus.MessageBus

	mu       sync.Mutex
	messages []model.Message
}

type SessionOption func(*Session)

func WithModelName(name string) SessionOption {
	return func(s *Session) { s.modelName = name }
}

func WithSystemPrompt(prompt string) SessionOption {
	return func(s *Session) { s.systemPrompt = prompt }
}

func WithMessageBus(b *bus.MessageBus) SessionOption {
	return func(s *Session) { s.bus = b }
}

func WithToolManager(tm *tool.Manager) SessionOption {
	return func(s *Session) { s.toolMgr = tm }
}

func WithTools(tools ...tool.Tool) SessionOption {
	return func(s *Session) {
		for _, t := range tools {
			s.toolMgr.Register(t)
		}
	}
}

func NewSession(provider model.Provider, opts ...SessionOption) *Session {
	s := &Session{
		provider: provider,
		toolMgr:  tool.NewManager(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Session) ProcessMessage(ctx context.Context, msg bus.InboundMessage) error {
	chatID := msg.ChatID
	sessionKey := msg.SessionKey

	if s.bus != nil {
		s.bus.PublishStream(bus.StreamMessage{
			Channel:    msg.Channel,
			ChatID:     chatID,
			SessionKey: sessionKey,
			Type:       bus.StreamEventStart,
		})
	}

	s.mu.Lock()

	if s.systemPrompt != "" && len(s.messages) == 0 {
		s.messages = append(s.messages, model.Message{
			Role:    "system",
			Content: s.systemPrompt,
		})
	}

	s.messages = append(s.messages, model.Message{
		Role:    "user",
		Content: msg.Content,
	})
	s.mu.Unlock()

	err := s.processLoop(ctx, msg.Channel, chatID, sessionKey)

	if s.bus != nil {
		s.bus.PublishStream(bus.StreamMessage{
			Channel:    msg.Channel,
			ChatID:     chatID,
			SessionKey: sessionKey,
			Type:       bus.StreamEventFinish,
		})
	}

	return err
}

func (s *Session) processLoop(ctx context.Context, channel, chatID, sessionKey string) error {
	iteration := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		iteration++
		if s.bus != nil {
			s.bus.PublishStream(bus.StreamMessage{
				Channel:    channel,
				ChatID:     chatID,
				SessionKey: sessionKey,
				Type:       bus.StreamEventStart,
				Iteration:  iteration,
			})
		}

		s.mu.Lock()
		req := &model.Request{
			Model:    s.modelName,
			Messages: s.messages,
			Tools:    s.toolMgr.Definitions(),
		}
		s.mu.Unlock()

		stream, err := s.provider.SendStream(ctx, req)
		if err != nil {
			if s.bus != nil {
				s.bus.PublishStream(bus.StreamMessage{
					Channel:    channel,
					ChatID:     chatID,
					SessionKey: sessionKey,
					Type:       bus.StreamEventError,
					Content:    err.Error(),
				})
			}
			return err
		}

		var assistantMsg strings.Builder
		var toolCalls []model.ToolCall
		var finishReason model.FinishReason
		var partID string = "main"

		if s.bus != nil {
			s.bus.PublishStream(bus.StreamMessage{
				Channel:    channel,
				ChatID:     chatID,
				SessionKey: sessionKey,
				Type:       bus.StreamEventTextStart,
				Delta:      partID,
			})
		}

		for event := range stream {
			if event.Delta != "" {
				assistantMsg.WriteString(event.Delta)
				if s.bus != nil {
					s.bus.PublishStream(bus.StreamMessage{
						Channel:    channel,
						ChatID:     chatID,
						SessionKey: sessionKey,
						Type:       bus.StreamEventTextDelta,
						Delta:      partID,
						Content:    event.Delta,
					})
				}
			}
			if event.ToolCall != nil {
				toolCalls = append(toolCalls, *event.ToolCall)
			}
			if event.FinishReason != "" {
				finishReason = event.FinishReason
			}
		}

		s.mu.Lock()
		msg := model.Message{
			Role:    "assistant",
			Content: assistantMsg.String(),
		}
		if len(toolCalls) > 0 {
			msg.ToolCalls = toolCalls
		}
		s.messages = append(s.messages, msg)
		s.mu.Unlock()

		if finishReason == model.FinishReasonToolCalls && len(toolCalls) > 0 {
			if err := s.executeToolCalls(ctx, channel, chatID, sessionKey, iteration, toolCalls); err != nil {
				return err
			}
			continue
		}

		return nil
	}
}

func (s *Session) executeToolCalls(ctx context.Context, channel, chatID, sessionKey string, iteration int, toolCalls []model.ToolCall) error {
	for _, tc := range toolCalls {
		var args map[string]interface{}
		if tc.Function.Arguments != "" {
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}

		if s.bus != nil {
			s.bus.PublishStream(bus.StreamMessage{
				Channel:    channel,
				ChatID:     chatID,
				SessionKey: sessionKey,
				Type:       bus.StreamEventToolCall,
				ToolName:   tc.Function.Name,
				ToolCallID: tc.ID,
				ToolArgs:   args,
				Iteration:  iteration,
			})
		}

		var argsJSON json.RawMessage
		if tc.Function.Arguments != "" {
			argsJSON = json.RawMessage(tc.Function.Arguments)
		}

		result, err := s.toolMgr.ExecuteWithContext(ctx, tc.Function.Name, argsJSON, channel, chatID)
		if err != nil {
			result = tool.ErrorResult(fmt.Sprintf("Error executing tool: %v", err))
		}

		var content string
		if result.IsError {
			content = fmt.Sprintf("Error: %s", result.Content)
			if s.bus != nil {
				s.bus.PublishStream(bus.StreamMessage{
					Channel:    channel,
					ChatID:     chatID,
					SessionKey: sessionKey,
					Type:       bus.StreamEventToolError,
					ToolCallID: tc.ID,
					Error:      result.Content,
				})
			}
		} else {
			content = result.Content
			if s.bus != nil {
				s.bus.PublishStream(bus.StreamMessage{
					Channel:    channel,
					ChatID:     chatID,
					SessionKey: sessionKey,
					Type:       bus.StreamEventToolResult,
					ToolCallID: tc.ID,
					ToolName:   tc.Function.Name,
					ToolResult: result.Content,
				})
			}
		}

		s.mu.Lock()
		s.messages = append(s.messages, model.Message{
			Role:       "tool",
			Content:    content,
			ToolCallID: tc.ID,
		})
		s.mu.Unlock()
	}

	return nil
}

func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
}

func (s *Session) Messages() []model.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]model.Message, len(s.messages))
	copy(result, s.messages)
	return result
}
