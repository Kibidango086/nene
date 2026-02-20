package tool

import (
	"context"
	"encoding/json"

	"github.com/nene-agent/nene/pkg/memory"
)

type MemoryStoreTool struct {
	parameters json.RawMessage
	mem        memory.Memory
	sessionID  string
}

func NewMemoryStoreTool(m memory.Memory) *MemoryStoreTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Unique identifier for this memory entry",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The information to store in long-term memory",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"core", "daily", "conversation"},
				"description": "Category of the memory: core (permanent), daily (temporary), or conversation (session-specific)",
			},
		},
		"required": []string{"key", "content"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &MemoryStoreTool{parameters: paramsJSON, mem: m}
}

func (t *MemoryStoreTool) SetSessionID(id string) { t.sessionID = id }

func (t *MemoryStoreTool) Name() string { return "memory_store" }
func (t *MemoryStoreTool) Description() string {
	return "Store information in long-term memory. Use this to remember important facts, user preferences, or context for future conversations."
}
func (t *MemoryStoreTool) Parameters() json.RawMessage { return t.parameters }

type memoryStoreArgs struct {
	Key      string `json:"key"`
	Content  string `json:"content"`
	Category string `json:"category"`
}

func (t *MemoryStoreTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	return nil, nil
}

func (t *MemoryStoreTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a memoryStoreArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	if a.Key == "" || a.Content == "" {
		return ErrorResult("key and content are required"), nil
	}

	req := &memory.StoreRequest{
		Key:      a.Key,
		Content:  a.Content,
		Category: memory.ParseCategory(a.Category),
	}
	if t.sessionID != "" {
		req.SessionID = t.sessionID
	}

	entry, err := t.mem.Store(ctx, req)
	if err != nil {
		return ErrorResult("failed to store memory: " + err.Error()), nil
	}

	return OkResult("Stored memory: " + entry.Key), nil
}
