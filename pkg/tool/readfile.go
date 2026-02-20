package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type ReadFileTool struct {
	parameters json.RawMessage
}

func NewReadFileTool() *ReadFileTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The path to the file to read",
			},
		},
		"required": []string{"path"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &ReadFileTool{parameters: paramsJSON}
}

func (t *ReadFileTool) Name() string                { return "read_file" }
func (t *ReadFileTool) Description() string         { return "Read the contents of a file" }
func (t *ReadFileTool) Parameters() json.RawMessage { return t.parameters }

type readFileArgs struct {
	Path string `json:"path"`
}

func (t *ReadFileTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	return NewApproval("Agent wants to read a file", "Read: "+a.Path), nil
}

func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	path := filepath.Clean(a.Path)
	if strings.Contains(path, "..") {
		return ErrorResult("path traversal not allowed"), nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return ErrorResult("failed to read file: " + err.Error()), nil
	}

	return OkResult(string(content)), nil
}
