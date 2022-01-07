package ratelimit

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis"
)

type RequestRate struct {
	ReplenishRate int
	Capacity      int
	Client        *redis.Client
}

var script *redis.Script

func init() {
	var src string = `
local tokens_key = KEYS[1]
local timestamp_key = KEYS[2]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])
local fill_time = capacity/rate
local ttl = math.floor(fill_time*2)

local last_tokens = tonumber(redis.call("get", tokens_key))
if last_tokens == nil then
  last_tokens = capacity
end

local last_refreshed = tonumber(redis.call("get", timestamp_key))
if last_refreshed == nil then
  last_refreshed = 0
end

local delta = math.max(0, now-last_refreshed)
local filled_tokens = math.min(capacity, last_tokens+(delta*rate))
local allowed = filled_tokens >= requested
local new_tokens = filled_tokens
if allowed then
  new_tokens = filled_tokens - requested
end

redis.call("setex", tokens_key, ttl, new_tokens)
redis.call("setex", timestamp_key, ttl, now)

return { allowed, new_tokens }
	`
	script = redis.NewScript(src)
}

func (rr *RequestRate) Limit(key string) (bool, error) {
	token := []string{"rrl", key, "token"}
	timestamp := []string{"rrl", key, "timestamp"}
	keys := []string{strings.Join(token, "."), strings.Join(timestamp, ".")}
	args := []interface{}{rr.ReplenishRate, rr.Capacity, time.Now().Unix(), 1}

	val, err := script.Run(rr.Client, keys, args...).Result()
	if err != nil {
		return false, err
	}
	fmt.Printf("response: %v\n", val)

	r, ok := val.([]interface{})
	if !ok {
		return false, errors.New("array response not returned")
	}

	// when allowed is false, redis returns nil
	if r[0] == nil {
		return false, nil
	}

	var i int64
	if i, ok = r[0].(int64); !ok {
		return false, errors.New("response could not be converted to int")
	}

	var allowed bool
	if i > 0 {
		allowed = true
	}
	return allowed, nil
}
