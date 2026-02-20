package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type SpawnTool struct {
	parameters json.RawMessage
	manager    *SubagentManager
	channel    string
	chatID     string
}

func NewSpawnTool(manager *SubagentManager) *SpawnTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tasks": map[string]interface{}{
				"type":        "array",
				"description": "Array of tasks to spawn in parallel. Each task runs independently.",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task": map[string]interface{}{
							"type":        "string",
							"description": "The task for subagent to complete",
						},
						"label": map[string]interface{}{
							"type":        "string",
							"description": "Unique label to identify this task result",
						},
					},
					"required": []string{"task"},
				},
			},
		},
		"required": []string{"tasks"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &SpawnTool{
		parameters: paramsJSON,
		manager:    manager,
	}
}

func (t *SpawnTool) Name() string { return "spawn" }
func (t *SpawnTool) Description() string {
	return `Spawn multiple subagents in parallel to handle independent tasks.
Each subagent runs concurrently and results are returned after all complete.
- Use "tasks" array to spawn multiple subagents at once
- Each task can have a "label" for identification
- All subagents run in parallel
- Perfect for: parallel searches, multiple file operations, dividing complex tasks`
}
func (t *SpawnTool) Parameters() json.RawMessage { return t.parameters }

func (t *SpawnTool) SetContext(channel, chatID string) {
	t.channel = channel
	t.chatID = chatID
}

type spawnTask struct {
	Task  string `json:"task"`
	Label string `json:"label"`
}

type spawnArgs struct {
	Tasks []spawnTask `json:"tasks"`
}

func (t *SpawnTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	var a spawnArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	taskCount := len(a.Tasks)
	return NewApproval("Agent wants to spawn subagents", fmt.Sprintf("Spawn %d parallel task(s)", taskCount)), nil
}

func (t *SpawnTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a spawnArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	if len(a.Tasks) == 0 {
		return ErrorResult("tasks array is required and must not be empty"), nil
	}

	if t.manager == nil {
		return ErrorResult("Subagent manager not configured"), nil
	}

	results := make([]SubagentResult, len(a.Tasks))
	var wg sync.WaitGroup

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i, task := range a.Tasks {
		wg.Add(1)
		label := task.Label
		if label == "" {
			label = fmt.Sprintf("task-%d", i+1)
		}

		go func(index int, taskStr, labelStr string) {
			defer wg.Done()
			result := t.manager.RunSync(subCtx, taskStr, labelStr)
			results[index] = result
		}(i, task.Task, label)
	}

	wg.Wait()

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Spawned %d subagent(s) in parallel:\n\n", len(a.Tasks)))

	for _, r := range results {
		if r.IsError {
			summary.WriteString(fmt.Sprintf("❌ %s: %s\n", r.Label, r.Content))
		} else {
			preview := r.Content
			if len(preview) > 300 {
				preview = preview[:300] + "..."
			}
			summary.WriteString(fmt.Sprintf("✅ %s (iterations: %d):\n%s\n\n", r.Label, r.Iteration, preview))
		}
	}

	return OkResult(summary.String()), nil
}
