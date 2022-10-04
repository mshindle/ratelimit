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

return { allowed, new_tokens }