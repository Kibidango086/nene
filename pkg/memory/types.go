package memory

import "time"

type Category string

const (
	CategoryCore         Category = "core"
	CategoryDaily        Category = "daily"
	CategoryConversation Category = "conversation"
)

type Entry struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Content   string    `json:"content"`
	Category  Category  `json:"category"`
	SessionID string    `json:"session_id,omitempty"`
	Score     float64   `json:"score,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type StoreRequest struct {
	Key       string   `json:"key"`
	Content   string   `json:"content"`
	Category  Category `json:"category"`
	SessionID string   `json:"session_id,omitempty"`
}

type RecallRequest struct {
	Query     string `json:"query"`
	Limit     int    `json:"limit"`
	SessionID string `json:"session_id,omitempty"`
}

type ListRequest struct {
	Category  Category `json:"category,omitempty"`
	SessionID string   `json:"session_id,omitempty"`
	Limit     int      `json:"limit,omitempty"`
}

func ParseCategory(s string) Category {
	switch s {
	case "core":
		return CategoryCore
	case "daily":
		return CategoryDaily
	case "conversation":
		return CategoryConversation
	default:
		if s != "" {
			return Category(s)
		}
		return CategoryCore
	}
}
