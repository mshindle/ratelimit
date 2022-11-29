package ratelimit

import (
	"context"
	"errors"
	"strings"
)

const defaultSep = "."

var (
	ErrCapacityReached     = errors.New("resource usage is at capacity")
	ErrInvalidLimitSetting = errors.New("limit configuration is invalid")
	ErrNoLimitSet          = errors.New("limit has not been set")
)

type RequestRate interface {
	SetLimit(ctx context.Context, resourceName, accountID string, limit int64, windowSec float64) error
	GetLimit(ctx context.Context, resourceName, accountID string) (int64, float64, error)
	GetToken(ctx context.Context, resourceName, accountID string) (bool, error)
}

// CreateKey joins multiple strings into a concise key for a hash map
func CreateKey(elems ...string) string {
	return strings.Join(elems, defaultSep)
}
