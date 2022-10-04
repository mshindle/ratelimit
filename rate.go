package ratelimit

import (
	"context"
	"errors"
)

var ErrCapacityReached = errors.New("resource usage is at capacity")

type RequestRate interface {
	SetLimit(ctx context.Context, resourceName, accountID string, limit int64, windowSec float64) error
	GetLimit(ctx context.Context, resourceName, accountID string) (int64, float64, error)
	GetToken(ctx context.Context, resourceName, accountID string) (bool, error)
}
