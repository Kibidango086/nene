package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/nene-agent/nene/pkg/bus"
)

type TelegramConfig struct {
	Token      string   `json:"token"`
	Proxy      string   `json:"proxy"`
	AllowFrom  []string `json:"allow_from"`
	StreamMode bool     `json:"stream_mode"`
}

type StreamState struct {
	messageID       int
	chatID          int64
	mu              sync.RWMutex
	parts           map[string]*Part
	toolCalls       map[string]*Part
	toolCallList    []string
	currentText     *Part
	iteration       int
	isStreaming     bool
	lastUpdate      time.Time
	lastMessageSent time.Time
}

type Part struct {
	ID         string
	Type       string
	Text       string
	ToolName   string
	ToolCallID string
	State      map[string]interface{}
}

func NewStreamState() *StreamState {
	return &StreamState{
		parts:           make(map[string]*Part),
		toolCalls:       make(map[string]*Part),
		toolCallList:    make([]string, 0),
		lastUpdate:      time.Now(),
		lastMessageSent: time.Time{},
	}
}

func (s *StreamState) SetMessageID(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageID = id
}

func (s *StreamState) SetLastMessageSent(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastMessageSent = t
}

func (s *StreamState) GetMessageID() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.messageID
}

func (s *StreamState) SetChatID(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chatID = id
}

func (s *StreamState) GetChatID() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chatID
}

func (s *StreamState) AddPart(part *Part) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.parts[part.ID] = part
}

func (s *StreamState) GetPart(id string) *Part {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.parts[id]
}

func (s *StreamState) SetCurrentText(part *Part) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentText = part
}

func (s *StreamState) UpdatePartDelta(partID string, delta string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if part, ok := s.parts[partID]; ok {
		part.Text += delta
		s.lastUpdate = time.Now()
	}
}

func (s *StreamState) AddToolCall(id string, part *Part) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolCalls[id] = part
	s.toolCallList = append(s.toolCallList, id)
}

func (s *StreamState) GetToolCall(id string) *Part {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.toolCalls[id]
}

func (s *StreamState) GetFinalText() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentText != nil && s.currentText.Text != "" {
		return s.currentText.Text
	}

	var finalText string
	for _, part := range s.parts {
		if part.Type == "text" && len(part.Text) > len(finalText) {
			finalText = part.Text
		}
	}
	return finalText
}

func (s *StreamState) GetDisplayContent() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var parts []string

	if s.iteration > 0 {
		parts = append(parts, fmt.Sprintf("🔄 Step %d", s.iteration))
	}

	if len(s.toolCalls) > 0 {
		var toolIDsToShow []string
		if len(s.toolCallList) <= 3 {
			toolIDsToShow = s.toolCallList
		} else {
			toolIDsToShow = s.toolCallList[len(s.toolCallList)-3:]
		}

		for _, toolID := range toolIDsToShow {
			tool := s.toolCalls[toolID]
			if tool == nil || tool.ToolName == "" {
				continue
			}

			var toolBlock strings.Builder
			toolHeader := fmt.Sprintf("🔧 %s", tool.ToolName)
			if status, ok := tool.State["status"].(string); ok {
				switch status {
				case "pending":
					toolHeader += " ⏳"
				case "running":
					toolHeader += " 🔄"
				case "completed":
					toolHeader += " ✅"
				case "error":
					toolHeader += " ❌"
				}
			}
			toolBlock.WriteString(toolHeader)
			toolBlock.WriteString("\n")

			if input, ok := tool.State["input"].(map[string]interface{}); ok && len(input) > 0 {
				argsJSON, _ := json.MarshalIndent(input, "", "  ")
				argsStr := string(argsJSON)
				maxInputLen := 200
				if len(argsStr) > maxInputLen {
					argsStr = argsStr[:maxInputLen] + "..."
				}
				toolBlock.WriteString("```\nInput:\n")
				toolBlock.WriteString(argsStr)
				toolBlock.WriteString("\n```")
			}

			if output, ok := tool.State["output"].(string); ok && output != "" {
				maxOutputLen := 150
				displayOutput := output
				if len(output) > maxOutputLen {
					displayOutput = output[:maxOutputLen] + "..."
				}
				toolBlock.WriteString("```\nOutput:\n")
				toolBlock.WriteString(displayOutput)
				toolBlock.WriteString("\n```")
			}

			parts = append(parts, toolBlock.String())
		}

		if len(s.toolCallList) > 3 {
			parts = append(parts, fmt.Sprintf("📋 ... and %d more", len(s.toolCallList)-3))
		}
	}

	finalText := ""
	if s.currentText != nil && s.currentText.Text != "" {
		finalText = s.currentText.Text
	} else {
		for _, part := range s.parts {
			if part.Type == "text" && len(part.Text) > len(finalText) {
				finalText = part.Text
			}
		}
	}

	if finalText != "" {
		if len(parts) > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, finalText)
	}

	return strings.Join(parts, "\n")
}

type TelegramChannel struct {
	*BaseChannel
	bot          *telego.Bot
	config       TelegramConfig
	streamStates sync.Map
	toolDetails  sync.Map
}

type ToolDetails struct {
	OriginalContent string
	Tools           []ToolDetailItem
}

type ToolDetailItem struct {
	ToolName string
	ToolID   string
	Input    map[string]interface{}
	Output   string
	Error    string
}

func NewTelegramChannel(cfg TelegramConfig, messageBus *bus.MessageBus) (*TelegramChannel, error) {
	var opts []telego.BotOption

	if cfg.Proxy != "" {
		proxyURL, parseErr := url.Parse(cfg.Proxy)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", cfg.Proxy, parseErr)
		}
		opts = append(opts, telego.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}))
	}

	bot, err := telego.NewBot(cfg.Token, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	base := NewBaseChannel("telegram", messageBus, cfg.AllowFrom)

	return &TelegramChannel{
		BaseChannel: base,
		bot:         bot,
		config:      cfg,
	}, nil
}

func (c *TelegramChannel) Start(ctx context.Context) error {
	updates, err := c.bot.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{
		Timeout: 30,
	})
	if err != nil {
		return fmt.Errorf("failed to start long polling: %w", err)
	}

	c.setRunning(true)
	fmt.Printf("Telegram bot connected: @%s\n", c.bot.Username())

	go c.handleStreamMessages(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case update, ok := <-updates:
				if !ok {
					fmt.Println("Updates channel closed")
					return
				}
				if update.Message != nil {
					c.handleMessage(ctx, update)
				} else if update.CallbackQuery != nil {
					c.handleCallbackQuery(ctx, update)
				}
			}
		}
	}()

	return nil
}

func (c *TelegramChannel) Stop(ctx context.Context) error {
	fmt.Println("Stopping Telegram bot...")
	c.setRunning(false)
	return nil
}

func (c *TelegramChannel) handleStreamMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, ok := c.bus.SubscribeStream(ctx)
			if !ok {
				continue
			}

			if msg.Channel != "telegram" {
				continue
			}

			c.handleStreamEvent(ctx, msg)
		}
	}
}

func (c *TelegramChannel) handleStreamEvent(ctx context.Context, msg bus.StreamMessage) {
	chatID, err := parseChatID(msg.ChatID)
	if err != nil {
		return
	}

	stateInterface, _ := c.streamStates.LoadOrStore(msg.ChatID, NewStreamState())
	state := stateInterface.(*StreamState)

	switch msg.Type {
	case bus.StreamEventTextStart:
		part := &Part{
			ID:   msg.Delta,
			Type: "text",
			Text: "",
		}
		state.AddPart(part)
		state.SetCurrentText(part)

	case bus.StreamEventTextDelta:
		if part := state.GetPart(msg.Delta); part != nil {
			state.UpdatePartDelta(msg.Delta, msg.Content)
		} else {
			state.UpdatePartDelta("main", msg.Content)
		}
		if state.lastMessageSent.IsZero() || time.Since(state.lastMessageSent) > 500*time.Millisecond {
			c.updateStreamMessage(ctx, chatID, state)
		}

	case bus.StreamEventTextEnd:
		c.updateStreamMessage(ctx, chatID, state)

	case bus.StreamEventToolCall:
		part := &Part{
			ID:         msg.ToolCallID,
			Type:       "tool",
			ToolName:   msg.ToolName,
			ToolCallID: msg.ToolCallID,
			State: map[string]interface{}{
				"status": "running",
				"input":  msg.ToolArgs,
			},
		}
		state.AddPart(part)
		state.AddToolCall(msg.ToolCallID, part)
		c.updateStreamMessage(ctx, chatID, state)

	case bus.StreamEventToolResult:
		if part := state.GetToolCall(msg.ToolCallID); part != nil {
			part.State["status"] = "completed"
			part.State["output"] = msg.ToolResult
		}
		c.updateStreamMessage(ctx, chatID, state)

	case bus.StreamEventToolError:
		if part := state.GetToolCall(msg.ToolCallID); part != nil {
			part.State["status"] = "error"
			part.State["error"] = msg.Error
		}
		c.updateStreamMessage(ctx, chatID, state)

	case bus.StreamEventFinish:
		c.finalizeStreamMessage(ctx, chatID, state)
		c.streamStates.Delete(msg.ChatID)

	case bus.StreamEventError:
		c.sendErrorMessage(ctx, chatID, msg.Content)
		c.streamStates.Delete(msg.ChatID)
	}
}

func (c *TelegramChannel) updateStreamMessage(ctx context.Context, chatID int64, state *StreamState) {
	content := state.GetDisplayContent()
	if content == "" {
		return
	}

	htmlContent := markdownToTelegramHTML(content)

	const maxLength = 4000
	if len(htmlContent) > maxLength {
		htmlContent = htmlContent[:maxLength] + "\n\n<i>[Message truncated]</i>"
	}

	messageID := state.GetMessageID()
	if messageID != 0 {
		editMsg := tu.EditMessageText(tu.ID(chatID), messageID, htmlContent)
		editMsg.ParseMode = telego.ModeHTML
		if _, err := c.bot.EditMessageText(ctx, editMsg); err != nil {
			c.sendNewStreamMessage(ctx, chatID, state, htmlContent)
		}
	} else {
		c.sendNewStreamMessage(ctx, chatID, state, htmlContent)
	}
}

func (c *TelegramChannel) sendNewStreamMessage(ctx context.Context, chatID int64, state *StreamState, htmlContent string) {
	msg := tu.Message(tu.ID(chatID), htmlContent)
	msg.ParseMode = telego.ModeHTML

	if oldMsgID := state.GetMessageID(); oldMsgID != 0 {
		c.bot.DeleteMessage(ctx, &telego.DeleteMessageParams{
			ChatID:    telego.ChatID{ID: chatID},
			MessageID: oldMsgID,
		})
	}

	sentMsg, err := c.bot.SendMessage(ctx, msg)
	if err != nil {
		msg.ParseMode = ""
		sentMsg, err = c.bot.SendMessage(ctx, msg)
		if err != nil {
			return
		}
	}

	state.SetMessageID(sentMsg.MessageID)
	state.SetChatID(chatID)
	state.SetLastMessageSent(time.Now())
}

func (c *TelegramChannel) finalizeStreamMessage(ctx context.Context, chatID int64, state *StreamState) {
	messageID := state.GetMessageID()
	finalContent := state.GetFinalText()

	if messageID != 0 {
		finalHTML := markdownToTelegramHTML(finalContent)
		const maxLength = 4000
		if len(finalHTML) > maxLength {
			finalHTML = finalHTML[:maxLength] + "\n\n<i>[Message truncated]</i>"
		}
		if finalHTML == "" {
			finalHTML = "✅ Completed"
		}

		editMsg := tu.EditMessageText(tu.ID(chatID), messageID, finalHTML)
		editMsg.ParseMode = telego.ModeHTML

		if len(state.toolCalls) > 0 {
			editMsg.ReplyMarkup = tu.InlineKeyboard(
				tu.InlineKeyboardRow(
					tu.InlineKeyboardButton("📋 View Details").WithCallbackData("view_details:0"),
				),
			)
		}

		if _, err := c.bot.EditMessageText(ctx, editMsg); err == nil {
			if len(state.toolCalls) > 0 {
				var tools []ToolDetailItem
				for toolID, tool := range state.toolCalls {
					item := ToolDetailItem{
						ToolName: tool.ToolName,
						ToolID:   toolID,
					}
					if input, ok := tool.State["input"].(map[string]interface{}); ok {
						item.Input = input
					}
					if output, ok := tool.State["output"].(string); ok {
						item.Output = output
					}
					if errMsg, ok := tool.State["error"].(string); ok {
						item.Error = errMsg
					}
					tools = append(tools, item)
				}
				c.toolDetails.Store(fmt.Sprintf("%d", messageID), &ToolDetails{
					OriginalContent: finalHTML,
					Tools:           tools,
				})
			}
		}
	} else {
		if finalContent != "" {
			c.sendNewStreamMessage(ctx, chatID, state, markdownToTelegramHTML(finalContent))
		}
	}
}

func (c *TelegramChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("telegram bot not running")
	}

	chatID, err := parseChatID(msg.ChatID)
	if err != nil {
		return fmt.Errorf("invalid chat ID: %w", err)
	}

	if msg.Content == "" {
		return nil
	}

	finalContent := markdownToTelegramHTML(msg.Content)

	const maxLength = 4000
	if len(finalContent) > maxLength {
		finalContent = finalContent[:maxLength] + "\n\n<i>[Message truncated]</i>"
	}

	tgMsg := tu.Message(tu.ID(chatID), finalContent)
	tgMsg.ParseMode = telego.ModeHTML

	if _, err := c.bot.SendMessage(ctx, tgMsg); err != nil {
		tgMsg.ParseMode = ""
		_, err = c.bot.SendMessage(ctx, tgMsg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *TelegramChannel) handleMessage(ctx context.Context, update telego.Update) {
	message := update.Message
	if message == nil {
		return
	}

	user := message.From
	if user == nil {
		return
	}

	userID := fmt.Sprintf("%d", user.ID)
	senderID := userID
	if user.Username != "" {
		senderID = fmt.Sprintf("%s|%s", userID, user.Username)
	}

	if !c.IsAllowed(userID) && !c.IsAllowed(senderID) {
		return
	}

	chatID := message.Chat.ID
	content := ""
	if message.Text != "" {
		content = message.Text
	}
	if message.Caption != "" {
		if content != "" {
			content += "\n"
		}
		content += message.Caption
	}

	if content == "" {
		return
	}

	c.bot.SendChatAction(ctx, tu.ChatAction(tu.ID(chatID), telego.ChatActionTyping))

	stateInterface, _ := c.streamStates.LoadOrStore(fmt.Sprintf("%d", chatID), NewStreamState())
	state := stateInterface.(*StreamState)
	state.SetChatID(chatID)

	metadata := map[string]string{
		"message_id": fmt.Sprintf("%d", message.MessageID),
		"user_id":    fmt.Sprintf("%d", user.ID),
		"username":   user.Username,
		"first_name": user.FirstName,
	}

	c.HandleMessage(senderID, fmt.Sprintf("%d", chatID), content, nil, metadata, c.config.StreamMode)
}

func (c *TelegramChannel) handleCallbackQuery(ctx context.Context, update telego.Update) {
	if update.CallbackQuery == nil {
		return
	}

	callback := update.CallbackQuery
	data := callback.Data

	if strings.HasPrefix(data, "view_details:") {
		msg := callback.Message
		if msg == nil {
			c.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
				CallbackQueryID: callback.ID,
				Text:            "Message not found",
				ShowAlert:       true,
			})
			return
		}

		chatID, messageID, ok := extractChatAndMessageID(msg)
		if !ok {
			c.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
				CallbackQueryID: callback.ID,
				Text:            "Cannot access message",
				ShowAlert:       true,
			})
			return
		}

		detailsInterface, ok := c.toolDetails.Load(fmt.Sprintf("%d", messageID))
		if !ok {
			c.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
				CallbackQueryID: callback.ID,
				Text:            "Details not found",
				ShowAlert:       true,
			})
			return
		}
		details := detailsInterface.(*ToolDetails)

		pageStr := strings.TrimPrefix(data, "view_details:")
		page := 0
		fmt.Sscanf(pageStr, "%d", &page)

		c.showToolDetailPage(ctx, chatID, int64(messageID), details, page, callback.ID)
	}
}

func (c *TelegramChannel) showToolDetailPage(ctx context.Context, chatID, messageID int64, details *ToolDetails, page int, callbackID string) {
	if page < 0 {
		page = 0
	}
	if page > len(details.Tools) {
		page = len(details.Tools)
	}

	var content string
	var keyboard *telego.InlineKeyboardMarkup

	if page == 0 {
		content = details.OriginalContent
		if len(details.Tools) > 0 {
			keyboard = tu.InlineKeyboard(
				tu.InlineKeyboardRow(
					tu.InlineKeyboardButton("📋 View Details").WithCallbackData("view_details:1"),
				),
			)
		}
	} else {
		toolIdx := page - 1
		if toolIdx >= len(details.Tools) {
			toolIdx = len(details.Tools) - 1
		}
		tool := details.Tools[toolIdx]

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("<b>🔧 Tool %d/%d: %s</b>\n", page, len(details.Tools), tool.ToolName))
		sb.WriteString(fmt.Sprintf("ID: <code>%s</code>\n", tool.ToolID))

		if tool.Input != nil && len(tool.Input) > 0 {
			argsJSON, _ := json.MarshalIndent(tool.Input, "", "  ")
			argsStr := string(argsJSON)
			if len(argsStr) > 1500 {
				argsStr = argsStr[:1500] + "\n...[truncated]"
			}
			sb.WriteString(fmt.Sprintf("\n<b>Input:</b>\n<pre>%s</pre>\n", argsStr))
		}

		if tool.Output != "" {
			output := tool.Output
			if len(output) > 1500 {
				output = output[:1500] + "\n...[truncated]"
			}
			sb.WriteString(fmt.Sprintf("\n<b>Output:</b>\n<pre>%s</pre>\n", output))
		}

		if tool.Error != "" {
			errMsg := tool.Error
			if len(errMsg) > 1000 {
				errMsg = errMsg[:1000] + "\n...[truncated]"
			}
			sb.WriteString(fmt.Sprintf("\n<b>Error:</b>\n<pre>%s</pre>\n", errMsg))
		}

		content = sb.String()

		var buttons []telego.InlineKeyboardButton
		if page > 1 {
			buttons = append(buttons, tu.InlineKeyboardButton("◀ Prev").WithCallbackData(fmt.Sprintf("view_details:%d", page-1)))
		}
		buttons = append(buttons, tu.InlineKeyboardButton("📋 Back").WithCallbackData("view_details:0"))
		if page < len(details.Tools) {
			buttons = append(buttons, tu.InlineKeyboardButton("Next ▶").WithCallbackData(fmt.Sprintf("view_details:%d", page+1)))
		}
		keyboard = tu.InlineKeyboard(tu.InlineKeyboardRow(buttons...))
	}

	editMsg := tu.EditMessageText(tu.ID(chatID), int(messageID), content)
	editMsg.ParseMode = telego.ModeHTML
	if keyboard != nil {
		editMsg.ReplyMarkup = keyboard
	}

	c.bot.EditMessageText(ctx, editMsg)

	c.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
		CallbackQueryID: callbackID,
	})
}

func extractChatAndMessageID(msg telego.MaybeInaccessibleMessage) (int64, int, bool) {
	switch m := msg.(type) {
	case *telego.Message:
		return m.Chat.ID, m.MessageID, true
	default:
		return 0, 0, false
	}
}

func (c *TelegramChannel) sendErrorMessage(ctx context.Context, chatID int64, errorMsg string) {
	htmlContent := markdownToTelegramHTML(fmt.Sprintf("❌ Error: %s", errorMsg))
	msg := tu.Message(tu.ID(chatID), htmlContent)
	msg.ParseMode = telego.ModeHTML
	c.bot.SendMessage(ctx, msg)
}

func parseChatID(chatIDStr string) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(chatIDStr, "%d", &id)
	return id, err
}

func markdownToTelegramHTML(text string) string {
	if text == "" {
		return ""
	}

	codeBlocks := extractCodeBlocks(text)
	text = codeBlocks.text

	inlineCodes := extractInlineCodes(text)
	text = inlineCodes.text

	text = regexp.MustCompile(`^#{1,6}\s+(.+)$`).ReplaceAllString(text, "<b>$1</b>")
	text = regexp.MustCompile(`^>\s*(.*)$`).ReplaceAllString(text, "<i>$1</i>")
	text = regexp.MustCompile(`^---+\s*$`).ReplaceAllString(text, "─"+strings.Repeat("─", 30))

	text = escapeHTML(text)

	text = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`).ReplaceAllString(text, `<a href="$2">$1</a>`)
	text = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(text, "<b>$1</b>")
	text = regexp.MustCompile(`__(.+?)__`).ReplaceAllString(text, "<b>$1</b>")
	text = regexp.MustCompile(`(^|[^\*])\*([^\*]+?)\*([^\*]|$)`).ReplaceAllString(text, "$1<i>$2</i>$3")
	text = regexp.MustCompile(`(^|[^_])_([^_]+?)_([^_]|$)`).ReplaceAllString(text, "$1<i>$2</i>$3")
	text = regexp.MustCompile(`~~(.+?)~~`).ReplaceAllString(text, "<s>$1</s>")
	text = regexp.MustCompile(`(?m)^[-*]\s+(.+)$`).ReplaceAllString(text, "• $1")
	text = regexp.MustCompile(`(?m)^(\d+)\.\s+(.+)$`).ReplaceAllString(text, "$1. $2")

	for i, code := range inlineCodes.codes {
		escaped := escapeHTML(code)
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00IC%d\x00", i), fmt.Sprintf("<code>%s</code>", escaped))
	}

	for i, code := range codeBlocks.codes {
		escaped := escapeHTML(code)
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00CB%d\x00", i), fmt.Sprintf("<pre><code>%s</code></pre>", escaped))
	}

	text = strings.ReplaceAll(text, "\n\n", "\n")

	return text
}

type codeBlockMatch struct {
	text  string
	codes []string
}

func extractCodeBlocks(text string) codeBlockMatch {
	re := regexp.MustCompile("```[\\w]*\\n?([\\s\\S]*?)```")
	matches := re.FindAllStringSubmatch(text, -1)

	codes := make([]string, 0, len(matches))
	for _, match := range matches {
		codes = append(codes, match[1])
	}

	i := 0
	text = re.ReplaceAllStringFunc(text, func(m string) string {
		placeholder := fmt.Sprintf("\x00CB%d\x00", i)
		i++
		return placeholder
	})

	return codeBlockMatch{text: text, codes: codes}
}

type inlineCodeMatch struct {
	text  string
	codes []string
}

func extractInlineCodes(text string) inlineCodeMatch {
	re := regexp.MustCompile("`([^`]+)`")
	matches := re.FindAllStringSubmatch(text, -1)

	codes := make([]string, 0, len(matches))
	for _, match := range matches {
		codes = append(codes, match[1])
	}

	i := 0
	text = re.ReplaceAllStringFunc(text, func(m string) string {
		placeholder := fmt.Sprintf("\x00IC%d\x00", i)
		i++
		return placeholder
	})

	return inlineCodeMatch{text: text, codes: codes}
}

func escapeHTML(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}
