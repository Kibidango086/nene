package tool

import (
	"context"
	"encoding/json"

	"github.com/nene-agent/nene/pkg/model"
)

type Approval struct {
	justification string
	what          string
	approved      bool
	rejected      bool
	reason        string
}

func NewApproval(justification, what string) *Approval {
	return &Approval{
		justification: justification,
		what:          what,
	}
}

func (a *Approval) Justification() string { return a.justification }
func (a *Approval) What() string          { return a.what }
func (a *Approval) IsApproved() bool      { return a.approved }
func (a *Approval) IsRejected() bool      { return a.rejected }
func (a *Approval) Reason() string        { return a.reason }

func (a *Approval) Approve() {
	a.approved = true
	a.rejected = false
}

func (a *Approval) Reject(reason string) {
	a.rejected = true
	a.approved = false
	a.reason = reason
}

type Result struct {
	Content string
	IsError bool
}

func NewResult(content string, isError bool) Result {
	return Result{Content: content, IsError: isError}
}

func OkResult(content string) Result {
	return NewResult(content, false)
}

func ErrorResult(content string) Result {
	return NewResult(content, true)
}

type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	MakeApproval(args json.RawMessage) (*Approval, error)
	Execute(ctx context.Context, args json.RawMessage) (Result, error)
}

type ContextualTool interface {
	Tool
	SetContext(channel, chatID string)
}

type Manager struct {
	tools map[string]Tool
}

func NewManager() *Manager {
	return &Manager{
		tools: make(map[string]Tool),
	}
}

func (m *Manager) Register(tool Tool) {
	m.tools[tool.Name()] = tool
}

func (m *Manager) Get(name string) (Tool, bool) {
	t, ok := m.tools[name]
	return t, ok
}

func (m *Manager) Definitions() []model.Tool {
	defs := make([]model.Tool, 0, len(m.tools))
	for _, t := range m.tools {
		defs = append(defs, model.NewFunctionTool(
			t.Name(),
			t.Description(),
			t.Parameters(),
		))
	}
	return defs
}

func (m *Manager) ExecuteWithContext(ctx context.Context, name string, args json.RawMessage, channel, chatID string) (Result, error) {
	tool, ok := m.Get(name)
	if !ok {
		return ErrorResult("unknown tool: " + name), nil
	}

	if contextualTool, ok := tool.(ContextualTool); ok && channel != "" && chatID != "" {
		contextualTool.SetContext(channel, chatID)
	}

	return tool.Execute(ctx, args)
}

func (m *Manager) Execute(ctx context.Context, name string, args json.RawMessage) (Result, error) {
	return m.ExecuteWithContext(ctx, name, args, "", "")
}

func (m *Manager) MakeApproval(name string, args json.RawMessage) (*Approval, error) {
	tool, ok := m.Get(name)
	if !ok {
		return nil, nil
	}
	return tool.MakeApproval(args)
}
