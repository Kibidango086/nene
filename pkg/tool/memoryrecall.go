package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nene-agent/nene/pkg/memory"
)

type MemoryRecallTool struct {
	parameters json.RawMessage
	mem        memory.Memory
	sessionID  string
}

func NewMemoryRecallTool(m memory.Memory) *MemoryRecallTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query to find relevant memories",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of memories to return (default 5)",
			},
		},
		"required": []string{"query"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &MemoryRecallTool{parameters: paramsJSON, mem: m}
}

func (t *MemoryRecallTool) SetSessionID(id string) { t.sessionID = id }

func (t *MemoryRecallTool) Name() string { return "memory_recall" }
func (t *MemoryRecallTool) Description() string {
	return "Search and retrieve relevant memories from long-term storage. Use this to recall previously stored information."
}
func (t *MemoryRecallTool) Parameters() json.RawMessage { return t.parameters }

type memoryRecallArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func (t *MemoryRecallTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	return nil, nil
}

func (t *MemoryRecallTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a memoryRecallArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	if a.Query == "" {
		return ErrorResult("query is required"), nil
	}

	req := &memory.RecallRequest{
		Query: a.Query,
		Limit: a.Limit,
	}
	if t.sessionID != "" {
		req.SessionID = t.sessionID
	}

	entries, err := t.mem.Recall(ctx, req)
	if err != nil {
		fmt.Printf("memory_recall error: %v\n", err)
		return ErrorResult("failed to recall memories: " + err.Error()), nil
	}

	fmt.Printf("memory_recall: query=%s, found=%d entries\n", a.Query, len(entries))

	if len(entries) == 0 {
		return OkResult("No relevant memories found."), nil
	}

	result := fmt.Sprintf("Found %d relevant memories:\n\n", len(entries))
	for i, e := range entries {
		result += fmt.Sprintf("%d. [%s] %s\n", i+1, e.Category, e.Key)
		result += fmt.Sprintf("   %s\n\n", e.Content)
	}

	return OkResult(result), nil
}
