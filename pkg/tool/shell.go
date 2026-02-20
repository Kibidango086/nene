package tool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
)

type ShellTool struct {
	parameters json.RawMessage
}

func NewShellTool() *ShellTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"cmdline": map[string]interface{}{
				"type":        "string",
				"description": "The command line to run",
			},
		},
		"required": []string{"cmdline"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &ShellTool{parameters: paramsJSON}
}

func (t *ShellTool) Name() string { return "shell" }
func (t *ShellTool) Description() string {
	return "Runs arbitrary commands like using a terminal. The command line should be single line if possible. Strings collected from stdout and stderr will be returned as the tool's output."
}
func (t *ShellTool) Parameters() json.RawMessage { return t.parameters }

type shellArgs struct {
	Cmdline string `json:"cmdline"`
}

func (t *ShellTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	var a shellArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	return NewApproval("Agent wants to run the command", a.Cmdline), nil
}

func (t *ShellTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a shellArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	var cmd *exec.Cmd
	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "cmd.exe"
		} else {
			shell = "/bin/sh"
		}
	}

	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, shell, "/c", a.Cmdline)
	} else {
		cmd = exec.CommandContext(ctx, shell, "-c", a.Cmdline)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ErrorResult(string(output) + "\nError: " + err.Error()), nil
	}

	return OkResult(string(output)), nil
}
