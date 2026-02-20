package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type ListFilesTool struct {
	parameters json.RawMessage
}

func NewListFilesTool() *ListFilesTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The directory path to list files from",
			},
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Optional glob pattern to filter files",
			},
		},
		"required": []string{"path"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &ListFilesTool{parameters: paramsJSON}
}

func (t *ListFilesTool) Name() string                { return "list_files" }
func (t *ListFilesTool) Description() string         { return "List files in a directory" }
func (t *ListFilesTool) Parameters() json.RawMessage { return t.parameters }

type listFilesArgs struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern,omitempty"`
}

func (t *ListFilesTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	var a listFilesArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	return NewApproval("Agent wants to list files", "List files in: "+a.Path), nil
}

func (t *ListFilesTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a listFilesArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	path := filepath.Clean(a.Path)
	if strings.Contains(path, "..") {
		return ErrorResult("path traversal not allowed"), nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return ErrorResult("failed to read directory: " + err.Error()), nil
	}

	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if a.Pattern != "" {
			matched, err := filepath.Match(a.Pattern, name)
			if err != nil {
				continue
			}
			if !matched {
				continue
			}
		}
		if entry.IsDir() {
			name += "/"
		}
		files = append(files, name)
	}

	return OkResult(strings.Join(files, "\n")), nil
}
