package rediskv

import (
	"io"

	"github.com/go-redis/redis/v8"
)

type RequestRateLimiter struct {
	client *redis.Client
	script *redis.Script
	debug  bool
	writer io.Writer
}

func NewRateLimiter(client *redis.Client, opts ...Option) *RequestRateLimiter {
	rrl := &RequestRateLimiter{
		client: client,
		script: redis.NewScript(requestRateScript),
	}
	for _, o := range opts {
		o(rrl)
	}

	return rrl
}

// Option represents a functional configuration of *DynamoDB
type Option func(rrl *RequestRateLimiter)

// WithDebug provides additional debugging information
func WithDebug(w io.Writer) Option {
	return func(rl *RequestRateLimiter) {
		rl.debug = true
		rl.writer = w
	}
}
