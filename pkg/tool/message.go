package tool

import (
	"context"
	"encoding/json"

	"github.com/nene-agent/nene/pkg/bus"
)

type MessageTool struct {
	parameters     json.RawMessage
	bus            *bus.MessageBus
	currentChannel string
	currentChatID  string
}

func NewMessageTool() *MessageTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The message content to send to the user",
			},
		},
		"required": []string{"content"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &MessageTool{parameters: paramsJSON}
}

func (t *MessageTool) SetBus(b *bus.MessageBus) {
	t.bus = b
}

func (t *MessageTool) SetContext(channel, chatID string) {
	t.currentChannel = channel
	t.currentChatID = chatID
}

func (t *MessageTool) Name() string { return "message" }
func (t *MessageTool) Description() string {
	return "Send a message to the user. Use this to communicate information, ask questions, or provide updates. The message will be sent immediately to the current chat."
}
func (t *MessageTool) Parameters() json.RawMessage { return t.parameters }

type messageArgs struct {
	Content string `json:"content"`
}

func (t *MessageTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	return nil, nil
}

func (t *MessageTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a messageArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	if a.Content == "" {
		return ErrorResult("content is required"), nil
	}

	if t.bus == nil || t.currentChannel == "" || t.currentChatID == "" {
		return ErrorResult("message tool not properly configured with channel context"), nil
	}

	t.bus.PublishOutbound(bus.OutboundMessage{
		Channel: t.currentChannel,
		ChatID:  t.currentChatID,
		Content: a.Content,
	})

	return OkResult("Message sent to user"), nil
}
