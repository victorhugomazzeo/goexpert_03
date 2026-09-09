local max    = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local block  = tonumber(ARGV[3])
local function retry_after(key)
	local ttl = redis.call('PTTL', key)
	if ttl < 0 then
		return 0
	end
	return ttl
end

local hits = redis.call('INCR', KEYS[1])

if hits == 1 then
	redis.call('PEXPIRE', KEYS[1], window)
end

if hits <= max then
	return {1, 0, hits, retry_after(KEYS[1])}
end

if hits == max + 1 then
	redis.call('PEXPIRE', KEYS[1], block)
	return {0, 0, hits, block}
end

return {0, 1, hits, retry_after(KEYS[1])}
