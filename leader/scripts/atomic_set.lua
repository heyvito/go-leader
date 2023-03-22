local key = KEYS[1]
local id = ARGV[1]
local ms = ARGV[2]

if (redis.call('SET', key, id, 'PX', ms, 'NX') == false) then
    return 0
else
    return 1
end
