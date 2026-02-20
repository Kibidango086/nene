package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/nene-agent/nene/pkg/model"
)

type SubagentManager struct {
	provider      model.Provider
	modelName     string
	toolMgr       *Manager
	systemPrompt  string
	maxIterations int
	mu            sync.RWMutex
}

func NewSubagentManager(provider model.Provider, modelName, systemPrompt string, toolMgr *Manager) *SubagentManager {
	return &SubagentManager{
		provider:      provider,
		modelName:     modelName,
		toolMgr:       toolMgr,
		systemPrompt:  systemPrompt,
		maxIterations: 10,
	}
}

type SubagentResult struct {
	Label     string
	Content   string
	IsError   bool
	Iteration int
}

func (sm *SubagentManager) RunSync(ctx context.Context, task, label string) SubagentResult {
	systemPrompt := `You are a subagent tasked with completing a specific task.
Complete the task independently and report a clear, concise result.
You have access to tools - use them as needed.
After completing the task, provide a summary of what was done.`

	messages := []model.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task},
	}

	iteration := 0
	var finalContent strings.Builder

	for iteration < sm.maxIterations {
		iteration++

		sm.mu.RLock()
		tools := sm.toolMgr.Definitions()
		sm.mu.RUnlock()

		req := &model.Request{
			Model:    sm.modelName,
			Messages: messages,
			Tools:    tools,
		}

		stream, err := sm.provider.SendStream(ctx, req)
		if err != nil {
			return SubagentResult{
				Label:   label,
				Content: fmt.Sprintf("Error: %v", err),
				IsError: true,
			}
		}

		var assistantMsg strings.Builder
		var toolCalls []model.ToolCall
		var finishReason model.FinishReason

		for event := range stream {
			if event.Delta != "" {
				assistantMsg.WriteString(event.Delta)
			}
			if event.ToolCall != nil {
				toolCalls = append(toolCalls, *event.ToolCall)
			}
			if event.FinishReason != "" {
				finishReason = event.FinishReason
			}
		}

		messages = append(messages, model.Message{
			Role:      "assistant",
			Content:   assistantMsg.String(),
			ToolCalls: toolCalls,
		})

		if finishReason != model.FinishReasonToolCalls || len(toolCalls) == 0 {
			finalContent.WriteString(assistantMsg.String())
			break
		}

		for _, tc := range toolCalls {
			var argsJSON json.RawMessage
			if tc.Function.Arguments != "" {
				argsJSON = json.RawMessage(tc.Function.Arguments)
			}

			result, err := sm.toolMgr.Execute(ctx, tc.Function.Name, argsJSON)
			if err != nil {
				result = ErrorResult(fmt.Sprintf("Error: %v", err))
			}

			content := result.Content
			if result.IsError {
				content = "Error: " + content
			}

			messages = append(messages, model.Message{
				Role:       "tool",
				Content:    content,
				ToolCallID: tc.ID,
			})
		}
	}

	return SubagentResult{
		Label:     label,
		Content:   finalContent.String(),
		IsError:   false,
		Iteration: iteration,
	}
}
