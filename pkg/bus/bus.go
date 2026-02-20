package bus

import (
	"context"
	"sync"
	"time"
)

type StreamEventType string

const (
	StreamEventTextStart  StreamEventType = "text-start"
	StreamEventTextDelta  StreamEventType = "text-delta"
	StreamEventTextEnd    StreamEventType = "text-end"
	StreamEventToolCall   StreamEventType = "tool-call"
	StreamEventToolResult StreamEventType = "tool-result"
	StreamEventToolError  StreamEventType = "tool-error"
	StreamEventStart      StreamEventType = "start"
	StreamEventFinish     StreamEventType = "finish"
	StreamEventError      StreamEventType = "error"
)

type InboundMessage struct {
	Channel    string
	SenderID   string
	ChatID     string
	Content    string
	Media      []string
	SessionKey string
	Metadata   map[string]string
	StreamMode bool
}

type OutboundMessage struct {
	Channel   string
	ChatID    string
	Content   string
	Media     []string
	SessionID string
}

type StreamMessage struct {
	Channel    string
	ChatID     string
	SessionKey string
	Type       StreamEventType
	Content    string
	Delta      string
	ToolName   string
	ToolArgs   map[string]interface{}
	ToolResult string
	ToolCallID string
	Error      string
	Iteration  int
	Timestamp  time.Time
}

type StreamHandler interface {
	OnStreamEvent(msg StreamMessage)
}

type MessageBus struct {
	inbound        chan InboundMessage
	outbound       chan OutboundMessage
	stream         chan StreamMessage
	handlers       map[string]func(context.Context, InboundMessage) error
	streamHandlers sync.Map
	mu             sync.RWMutex
}

func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:  make(chan InboundMessage, 100),
		outbound: make(chan OutboundMessage, 100),
		stream:   make(chan StreamMessage, 100),
		handlers: make(map[string]func(context.Context, InboundMessage) error),
	}
}

func (mb *MessageBus) PublishInbound(msg InboundMessage) {
	mb.inbound <- msg
}

func (mb *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, bool) {
	select {
	case msg := <-mb.inbound:
		return msg, true
	case <-ctx.Done():
		return InboundMessage{}, false
	}
}

func (mb *MessageBus) PublishOutbound(msg OutboundMessage) {
	mb.outbound <- msg
}

func (mb *MessageBus) SubscribeOutbound(ctx context.Context) (OutboundMessage, bool) {
	select {
	case msg := <-mb.outbound:
		return msg, true
	case <-ctx.Done():
		return OutboundMessage{}, false
	}
}

func (mb *MessageBus) PublishStream(msg StreamMessage) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	mb.stream <- msg
}

func (mb *MessageBus) SubscribeStream(ctx context.Context) (StreamMessage, bool) {
	select {
	case msg := <-mb.stream:
		return msg, true
	case <-ctx.Done():
		return StreamMessage{}, false
	}
}

func (mb *MessageBus) RegisterStreamHandler(chatID string, handler StreamHandler) {
	mb.streamHandlers.Store(chatID, handler)
}

func (mb *MessageBus) UnregisterStreamHandler(chatID string) {
	mb.streamHandlers.Delete(chatID)
}

func (mb *MessageBus) RegisterHandler(channel string, handler func(context.Context, InboundMessage) error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.handlers[channel] = handler
}

func (mb *MessageBus) GetHandler(channel string) (func(context.Context, InboundMessage) error, bool) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	handler, ok := mb.handlers[channel]
	return handler, ok
}

func (mb *MessageBus) Close() {
	close(mb.inbound)
	close(mb.outbound)
	close(mb.stream)
}
