package tool

import (
	"context"
	"encoding/json"
)

type ThinkTool struct {
	parameters json.RawMessage
}

func NewThinkTool() *ThinkTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"thought": map[string]interface{}{
				"type":        "string",
				"description": "Your internal reasoning and thought process",
			},
		},
		"required": []string{"thought"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &ThinkTool{parameters: paramsJSON}
}

func (t *ThinkTool) Name() string { return "think" }
func (t *ThinkTool) Description() string {
	return "Use this tool to think through complex problems step by step. Your thought process will be recorded but not shown to the user. This helps you organize your reasoning before taking action."
}
func (t *ThinkTool) Parameters() json.RawMessage { return t.parameters }

type thinkArgs struct {
	Thought string `json:"thought"`
}

func (t *ThinkTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	return nil, nil
}

func (t *ThinkTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a thinkArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	return OkResult("Thought recorded: " + a.Thought), nil
}
