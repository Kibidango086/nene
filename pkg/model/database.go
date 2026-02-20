package model

import (
	"encoding/json"
	"sync"
)

type ModelDatabase struct {
	mu        sync.RWMutex
	providers map[string]*ProviderInfo
	models    map[string]*ModelInfo
}

func NewModelDatabase() *ModelDatabase {
	return &ModelDatabase{
		providers: make(map[string]*ProviderInfo),
		models:    make(map[string]*ModelInfo),
	}
}

func (db *ModelDatabase) AddProvider(info *ProviderInfo) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.providers[info.ID] = info
	for modelID, model := range info.Models {
		key := info.ID + "/" + modelID
		db.models[key] = model
	}
}

func (db *ModelDatabase) GetProvider(id string) (*ProviderInfo, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	info, ok := db.providers[id]
	return info, ok
}

func (db *ModelDatabase) GetModel(providerID, modelID string) (*ModelInfo, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	key := providerID + "/" + modelID
	info, ok := db.models[key]
	return info, ok
}

func (db *ModelDatabase) ListProviders() []string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	ids := make([]string, 0, len(db.providers))
	for id := range db.providers {
		ids = append(ids, id)
	}
	return ids
}

func (db *ModelDatabase) ListModels(providerID string) []*ModelInfo {
	db.mu.RLock()
	defer db.mu.RUnlock()
	models := make([]*ModelInfo, 0)
	for key, m := range db.models {
		if providerID == "" || m.ProviderID == providerID {
			_ = key
			models = append(models, m)
		}
	}
	return models
}

func (db *ModelDatabase) LoadFromJSON(data []byte) error {
	var providers map[string]*ProviderInfo
	if err := json.Unmarshal(data, &providers); err != nil {
		return err
	}
	for _, info := range providers {
		db.AddProvider(info)
	}
	return nil
}

var defaultDB = NewModelDatabase()

func DefaultModelDatabase() *ModelDatabase {
	return defaultDB
}

func GetBuiltinProviders() map[string]*ProviderInfo {
	return map[string]*ProviderInfo{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Env:  []string{"OPENAI_API_KEY"},
			Models: map[string]*ModelInfo{
				"gpt-4o": {
					ID:         "gpt-4o",
					ProviderID: "openai",
					Name:       "GPT-4o",
					Family:     "gpt-4",
					Status:     "active",
					Capabilities: Capabilities{
						Temperature: true,
						Reasoning:   false,
						Attachment:  true,
						ToolCall:    true,
					},
					Cost:  Cost{Input: 2.5, Output: 10},
					Limit: Limit{Context: 128000, Output: 16384},
				},
				"gpt-4o-mini": {
					ID:         "gpt-4o-mini",
					ProviderID: "openai",
					Name:       "GPT-4o Mini",
					Family:     "gpt-4",
					Status:     "active",
					Capabilities: Capabilities{
						Temperature: true,
						Reasoning:   false,
						Attachment:  true,
						ToolCall:    true,
					},
					Cost:  Cost{Input: 0.15, Output: 0.6},
					Limit: Limit{Context: 128000, Output: 16384},
				},
				"gpt-4-turbo": {
					ID:         "gpt-4-turbo",
					ProviderID: "openai",
					Name:       "GPT-4 Turbo",
					Family:     "gpt-4",
					Status:     "active",
					Capabilities: Capabilities{
						Temperature: true,
						Reasoning:   false,
						Attachment:  true,
						ToolCall:    true,
					},
					Cost:  Cost{Input: 10, Output: 30},
					Limit: Limit{Context: 128000, Output: 4096},
				},
			},
		},
		"anthropic": {
			ID:   "anthropic",
			Name: "Anthropic",
			Env:  []string{"ANTHROPIC_API_KEY"},
			Models: map[string]*ModelInfo{
				"claude-sonnet-4-20250514": {
					ID:         "claude-sonnet-4-20250514",
					ProviderID: "anthropic",
					Name:       "Claude Sonnet 4",
					Family:     "claude",
					Status:     "active",
					Capabilities: Capabilities{
						Temperature: true,
						Reasoning:   true,
						Attachment:  true,
						ToolCall:    true,
					},
					Cost:  Cost{Input: 3, Output: 15},
					Limit: Limit{Context: 200000, Output: 8192},
				},
				"claude-3-5-sonnet-20241022": {
					ID:         "claude-3-5-sonnet-20241022",
					ProviderID: "anthropic",
					Name:       "Claude 3.5 Sonnet",
					Family:     "claude",
					Status:     "active",
					Capabilities: Capabilities{
						Temperature: true,
						Reasoning:   false,
						Attachment:  true,
						ToolCall:    true,
					},
					Cost:  Cost{Input: 3, Output: 15},
					Limit: Limit{Context: 200000, Output: 8192},
				},
				"claude-3-5-haiku-20241022": {
					ID:         "claude-3-5-haiku-20241022",
					ProviderID: "anthropic",
					Name:       "Claude 3.5 Haiku",
					Family:     "claude",
					Status:     "active",
					Capabilities: Capabilities{
						Temperature: true,
						Reasoning:   false,
						Attachment:  true,
						ToolCall:    true,
					},
					Cost:  Cost{Input: 0.8, Output: 4},
					Limit: Limit{Context: 200000, Output: 8192},
				},
			},
		},
		"azure": {
			ID:     "azure",
			Name:   "Azure OpenAI",
			Env:    []string{"AZURE_OPENAI_API_KEY"},
			Models: map[string]*ModelInfo{},
		},
	}
}

func init() {
	for _, info := range GetBuiltinProviders() {
		defaultDB.AddProvider(info)
	}
}
