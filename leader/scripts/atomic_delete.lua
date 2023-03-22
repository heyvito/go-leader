local key = KEYS[1]
local id = ARGV[1]

if (redis.call('GET', key) == id) then
  redis.call('DEL', key)
  return 1
else
  return 0
end
