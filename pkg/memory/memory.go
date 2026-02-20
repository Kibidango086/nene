package memory

import (
	"context"
)

type Memory interface {
	Store(ctx context.Context, req *StoreRequest) (*Entry, error)
	Recall(ctx context.Context, req *RecallRequest) ([]*Entry, error)
	Get(ctx context.Context, key string) (*Entry, error)
	List(ctx context.Context, req *ListRequest) ([]*Entry, error)
	Forget(ctx context.Context, key string) (bool, error)
	Count(ctx context.Context) (int, error)
	Close() error
}
