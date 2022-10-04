package rediskv

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mshindle/ratelimit"
)

const (
	limitKey          = "limit"
	tokenKey          = "token"
	timestampKey      = "timestamp"
	windowKey         = "window_sec"
	requestRateScript = `
local tokens_key = KEYS[1]
local timestamp_key = KEYS[2]

local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

local last_tokens = tonumber(redis.call("get", tokens_key))
if last_tokens == nil then
  last_tokens = capacity
end

local last_refreshed = tonumber(redis.call("get", timestamp_key))
if last_refreshed == nil then
  last_refreshed = 0
end

local delta = math.max(0, now-last_refreshed)
local filled_tokens = math.min(capacity, last_tokens+(delta/rate))
local allowed = filled_tokens >= requested
local new_tokens = filled_tokens
if allowed then
  new_tokens = filled_tokens - requested
end

redis.call("set", tokens_key, new_tokens)
redis.call("set", timestamp_key, now)

return { allowed, new_tokens, tostring(last_tokens), tostring(filled_tokens) }
`
)

func (rrl *RequestRateLimiter) SetLimit(ctx context.Context, resourceName, accountID string, limit int64, windowSec float64) error {
	key := strings.Join([]string{resourceName, accountID, limitKey}, ".")
	fields := map[string]interface{}{
		limitKey:  limit,
		windowKey: windowSec,
	}
	_, err := rrl.client.HSet(ctx, key, fields).Result()
	return err
}

func (rrl *RequestRateLimiter) GetLimit(ctx context.Context, resourceName, accountID string) (limit int64, window float64, err error) {
	key := strings.Join([]string{resourceName, accountID, limitKey}, ".")
	res, err := rrl.client.HGetAll(ctx, key).Result()
	if err != nil {
		return
	}

	if limit, err = strconv.ParseInt(res[limitKey], 10, 64); err != nil {
		err = ratelimit.ErrInvalidLimitSetting
		return
	}
	if window, err = strconv.ParseFloat(res[windowKey], 64); err != nil {
		err = ratelimit.ErrInvalidLimitSetting
		return
	}

	if rrl.debug {
		_, _ = fmt.Fprintf(rrl.writer, "returned map: %v", res)
	}
	return
}

func (rrl *RequestRateLimiter) GetToken(ctx context.Context, resourceName, accountID string) (bool, error) {
	// grab the current limits
	limit, window, err := rrl.GetLimit(ctx, resourceName, accountID)
	if err != nil {
		return false, err
	}
	windowMS := window * 1000
	msToken := int64(math.Max(1.0, windowMS/float64(limit))) // number of milliseconds to accumulate a new token

	token := []string{resourceName, accountID, tokenKey}
	timestamp := []string{resourceName, accountID, timestampKey}
	keys := []string{strings.Join(token, "."), strings.Join(timestamp, ".")}
	args := []interface{}{msToken, limit, time.Now().UnixMilli(), 1}

	if rrl.debug {
		_, _ = fmt.Fprintf(rrl.writer, "keys: %v", keys)
		_, _ = fmt.Fprintf(rrl.writer, "args: %v", args)
	}

	val, err := rrl.script.Run(ctx, rrl.client, keys, args...).Result()
	if err != nil {
		return false, err
	}
	if rrl.debug {
		_, _ = fmt.Fprintf(rrl.writer, "response: %v", val)
	}

	r, ok := val.([]interface{})
	if !ok {
		return false, errors.New("array response not returned")
	}

	// when allowed is false, redis returns nil
	if r[0] == nil {
		return false, nil
	}
	return true, nil
}
