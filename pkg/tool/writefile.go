package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type WriteFileTool struct {
	parameters json.RawMessage
}

func NewWriteFileTool() *WriteFileTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The path to the file to write",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &WriteFileTool{parameters: paramsJSON}
}

func (t *WriteFileTool) Name() string                { return "write_file" }
func (t *WriteFileTool) Description() string         { return "Write content to a file" }
func (t *WriteFileTool) Parameters() json.RawMessage { return t.parameters }

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *WriteFileTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	var a writeFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	preview := a.Content
	if len(preview) > 100 {
		preview = preview[:100] + "..."
	}
	return NewApproval("Agent wants to write to a file", "Write to "+a.Path+":\n"+preview), nil
}

func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a writeFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	path := filepath.Clean(a.Path)
	if strings.Contains(path, "..") {
		return ErrorResult("path traversal not allowed"), nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ErrorResult("failed to create directory: " + err.Error()), nil
	}

	if err := os.WriteFile(path, []byte(a.Content), 0644); err != nil {
		return ErrorResult("failed to write file: " + err.Error()), nil
	}

	return OkResult("File written successfully: " + path), nil
}
