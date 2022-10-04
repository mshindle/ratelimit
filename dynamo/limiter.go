package dynamo

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

const (
	defaultLimitRate    = 5
	defaultWindowSecond = 1.0
	tokenKey            = "resourceName"
	tokenRange          = "accountId"
	limitKey            = "resourceName"
	limitRange          = "accountId"
	limitSecondary      = "serviceName"
)

type RateLimiter struct {
	config      aws.Config
	client      *dynamodb.Client
	serviceName string
	tokenTable  string
	tokenKey    string
	tokenRange  string
	limitTable  string
	limitKey    string
	limitRange  string
	debug       bool
	writer      io.Writer
}

func NewRateLimiter(ctx context.Context, opts ...Option) (*RateLimiter, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	rl := &RateLimiter{
		config:     cfg,
		tokenTable: "token",
		limitTable: "limit",
	}

	for _, o := range opts {
		o(rl)
	}

	if rl.client == nil {
		rl.client = dynamodb.NewFromConfig(rl.config)
	}

	return rl, nil
}

// Option represents a functional configuration of *DynamoDB
type Option func(rl *RateLimiter)

// WithConfig allows the caller to use a custom AWS Configuration
func WithConfig(cfg aws.Config) Option {
	return func(rl *RateLimiter) {
		rl.config = cfg
	}
}

// WithDebug provides additional debugging information
func WithDebug(w io.Writer) Option {
	return func(rl *RateLimiter) {
		rl.debug = true
		rl.writer = w
	}
}

// WithClient allows the caller to provide a custom dynamodb.Client
func WithClient(client *dynamodb.Client) Option {
	return func(rl *RateLimiter) {
		rl.client = client
	}
}

// WithServiceName specifies the service this limiter works against
func WithServiceName(name string) Option {
	return func(rl *RateLimiter) {
		rl.serviceName = name
	}
}

// WithTokenTable specifies the token tables
func WithTokenTable(tokenTable string) Option {
	return func(rl *RateLimiter) {
		rl.tokenTable = tokenTable
	}
}

// WithLimitTable specifies the limits table
func WithLimitTable(limitTable string) Option {
	return func(rl *RateLimiter) {
		rl.limitTable = limitTable
	}
}
