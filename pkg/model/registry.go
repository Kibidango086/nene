package model

import (
	"context"
	"fmt"
	"sync"
)

type ProviderFactory func(config ProviderConfig) (Provider, error)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	factories map[string]ProviderFactory
	infos     map[string]*ProviderInfo
	models    map[string]*ModelInfo
	defaultID string
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		factories: make(map[string]ProviderFactory),
		infos:     make(map[string]*ProviderInfo),
		models:    make(map[string]*ModelInfo),
	}
}

func (r *Registry) RegisterFactory(id string, factory ProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[id] = factory
}

func (r *Registry) RegisterProvider(id string, provider Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[id] = provider
	if r.defaultID == "" {
		r.defaultID = id
	}
}

func (r *Registry) RegisterInfo(info *ProviderInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.infos[info.ID] = info
	for modelID, model := range info.Models {
		r.models[info.ID+"/"+modelID] = model
	}
}

func (r *Registry) GetProvider(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

func (r *Registry) GetInfo(id string) (*ProviderInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.infos[id]
	return info, ok
}

func (r *Registry) GetModel(providerID, modelID string) (*ModelInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	model, ok := r.models[providerID+"/"+modelID]
	return model, ok
}

func (r *Registry) CreateProvider(config ProviderConfig) (Provider, error) {
	r.mu.RLock()
	factory, ok := r.factories[config.ID]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider factory not found: %s", config.ID)
	}

	provider, err := factory(config)
	if err != nil {
		return nil, err
	}

	r.RegisterProvider(config.ID, provider)
	return provider, nil
}

func (r *Registry) DefaultProvider() (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.defaultID == "" {
		return nil, false
	}
	p, ok := r.providers[r.defaultID]
	return p, ok
}

func (r *Registry) SetDefault(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultID = id
}

func (r *Registry) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	return ids
}

func (r *Registry) ListModels(providerID string) []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	models := make([]*ModelInfo, 0)
	for key, model := range r.models {
		if providerID == "" || model.ProviderID == providerID {
			models = append(models, r.models[key])
		}
	}
	return models
}

func (r *Registry) Send(ctx context.Context, providerID string, req *Request) (*Response, error) {
	provider, ok := r.GetProvider(providerID)
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerID)
	}
	return provider.Send(ctx, req)
}

func (r *Registry) SendStream(ctx context.Context, providerID string, req *Request) (<-chan *ResponseEvent, error) {
	provider, ok := r.GetProvider(providerID)
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerID)
	}
	return provider.SendStream(ctx, req)
}

var globalRegistry = NewRegistry()

func DefaultRegistry() *Registry {
	return globalRegistry
}

func RegisterFactory(id string, factory ProviderFactory) {
	globalRegistry.RegisterFactory(id, factory)
}

func RegisterProvider(id string, provider Provider) {
	globalRegistry.RegisterProvider(id, provider)
}

func GetProvider(id string) (Provider, bool) {
	return globalRegistry.GetProvider(id)
}

func CreateProvider(config ProviderConfig) (Provider, error) {
	return globalRegistry.CreateProvider(config)
}
