local key = KEYS[1]
local id = ARGV[1]
local ms = ARGV[2]

if (id == redis.call('GET', key)) then
  redis.call('PEXPIRE', key, ms)
  return 1
else
  return 0
end
