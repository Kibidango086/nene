package tool

import (
	"context"
	"encoding/json"

	"github.com/nene-agent/nene/pkg/memory"
)

type MemoryForgetTool struct {
	parameters json.RawMessage
	mem        memory.Memory
}

func NewMemoryForgetTool(m memory.Memory) *MemoryForgetTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "The key of the memory entry to delete",
			},
		},
		"required": []string{"key"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &MemoryForgetTool{parameters: paramsJSON, mem: m}
}

func (t *MemoryForgetTool) Name() string { return "memory_forget" }
func (t *MemoryForgetTool) Description() string {
	return "Delete a memory entry from long-term storage. Use this to remove outdated or incorrect information."
}
func (t *MemoryForgetTool) Parameters() json.RawMessage { return t.parameters }

type memoryForgetAllArgs struct {
	Key string `json:"key"`
}

func (t *MemoryForgetTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	return nil, nil
}

func (t *MemoryForgetTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a memoryForgetAllArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	if a.Key == "" {
		return ErrorResult("key is required"), nil
	}

	deleted, err := t.mem.Forget(ctx, a.Key)
	if err != nil {
		return ErrorResult("failed to forget memory: " + err.Error()), nil
	}

	if deleted {
		return OkResult("Memory '" + a.Key + "' has been forgotten."), nil
	}
	return OkResult("Memory '" + a.Key + "' was not found."), nil
}
