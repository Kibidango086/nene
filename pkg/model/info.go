package model

import "encoding/json"

type Capabilities struct {
	Temperature bool `json:"temperature"`
	Reasoning   bool `json:"reasoning"`
	Attachment  bool `json:"attachment"`
	ToolCall    bool `json:"tool_call"`
	Input       struct {
		Text  bool `json:"text"`
		Audio bool `json:"audio"`
		Image bool `json:"image"`
		Video bool `json:"video"`
		PDF   bool `json:"pdf"`
	} `json:"input"`
	Output struct {
		Text  bool `json:"text"`
		Audio bool `json:"audio"`
		Image bool `json:"image"`
		Video bool `json:"video"`
		PDF   bool `json:"pdf"`
	} `json:"output"`
}

type Cost struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
	Cache  struct {
		Read  float64 `json:"read"`
		Write float64 `json:"write"`
	} `json:"cache"`
}

type Limit struct {
	Context int `json:"context"`
	Input   int `json:"input,omitempty"`
	Output  int `json:"output"`
}

type ModelInfo struct {
	ID           string          `json:"id"`
	ProviderID   string          `json:"provider_id"`
	Name         string          `json:"name"`
	Family       string          `json:"family,omitempty"`
	Capabilities Capabilities    `json:"capabilities"`
	Cost         Cost            `json:"cost"`
	Limit        Limit           `json:"limit"`
	Status       string          `json:"status"` // active, alpha, beta, deprecated
	Options      json.RawMessage `json:"options,omitempty"`
}

type ProviderInfo struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Env     []string               `json:"env"`
	Options map[string]interface{} `json:"options"`
	Models  map[string]*ModelInfo  `json:"models"`
}

type ProviderConfig struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	Timeout   int    `json:"timeout"`
	MaxTokens int    `json:"max_tokens"`
}
