package network

import (
	"context"
)

type Api interface {
	GetBlockCount(ctx context.Context) (int, error)
}
