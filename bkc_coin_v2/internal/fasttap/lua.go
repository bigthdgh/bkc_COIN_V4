package fasttap

// Lua script notes:
// - Uses user hash for energy state (float), boost params, updated_at timestamp
// - Uses daily hash for counters (tapped, extra_quota)
// - Uses system hash for reserve_supply / reserved_supply checks
// - Emits a compact event to a Redis Stream for async persistence
const tapLua = `
local userKey = KEYS[1]
local dailyKey = KEYS[2]
local sysKey = KEYS[3]
local streamKey = KEYS[4]

local now = tonumber(ARGV[1])
local requested = tonumber(ARGV[2])
local baseRegen = tonumber(ARGV[3])
local dailyLimit = tonumber(ARGV[4])
local defaultEnergyMax = tonumber(ARGV[5])
local streamMaxLen = tonumber(ARGV[6])
local coinPerTap = tonumber(ARGV[7])
local userID = tostring(ARGV[8])
local dayStr = tostring(ARGV[9])
local dailyTTL = tonumber(ARGV[10])

if requested == nil or requested <= 0 then requested = 1 end
if baseRegen == nil or baseRegen < 0 then baseRegen = 0 end
if dailyLimit == nil then dailyLimit = 0 end
if defaultEnergyMax == nil or defaultEnergyMax < 0 then defaultEnergyMax = 0 end
if streamMaxLen == nil or streamMaxLen < 10000 then streamMaxLen = 10000 end
if coinPerTap == nil or coinPerTap <= 0 then coinPerTap = 1 end
if dailyTTL == nil or dailyTTL <= 0 then dailyTTL = 259200 end

-- Load user energy state.
local energy = tonumber(redis.call('HGET', userKey, 'energy') or '0')
local energyMax = tonumber(redis.call('HGET', userKey, 'energy_max') or tostring(defaultEnergyMax))
local updatedAt = tonumber(redis.call('HGET', userKey, 'energy_updated_at') or tostring(now))

local boostUntil = tonumber(redis.call('HGET', userKey, 'boost_until') or '0')
local regenMult = tonumber(redis.call('HGET', userKey, 'boost_regen_mult') or '1')
local maxMult = tonumber(redis.call('HGET', userKey, 'boost_max_mult') or '1')

if energy == nil then energy = 0 end
if energyMax == nil then energyMax = defaultEnergyMax end
if updatedAt == nil then updatedAt = now end
if boostUntil == nil then boostUntil = 0 end
if regenMult == nil or regenMult <= 0 then regenMult = 1 end
if maxMult == nil or maxMult <= 0 then maxMult = 1 end

local eMax = energyMax
local eRegen = baseRegen
if now < boostUntil then
  eMax = eMax * maxMult
  eRegen = eRegen * regenMult
end
if eMax < 0 then eMax = 0 end
if eRegen < 0 then eRegen = 0 end

-- Regen energy.
local dt = now - updatedAt
if dt > 0 and eRegen > 0 then
  energy = energy + (dt * eRegen)
  if energy > eMax then energy = eMax end
end
if energy < 0 then energy = 0 end

local mintable = math.floor(energy)
if mintable < 0 then mintable = 0 end

-- Daily quota.
local tapped = tonumber(redis.call('HGET', dailyKey, 'tapped') or '0')
local extraQuota = tonumber(redis.call('HGET', dailyKey, 'extra_quota') or '0')
if tapped == nil then tapped = 0 end
if extraQuota == nil then extraQuota = 0 end

local remainingDaily = 9223372036854775807
if dailyLimit ~= nil and dailyLimit > 0 then
  remainingDaily = (dailyLimit + extraQuota) - tapped
  if remainingDaily < 0 then remainingDaily = 0 end
end

-- System reserve (respect reserved_supply).
local reserve = tonumber(redis.call('HGET', sysKey, 'reserve_supply') or '0')
local reserved = tonumber(redis.call('HGET', sysKey, 'reserved_supply') or '0')
if reserve == nil then reserve = 0 end
if reserved == nil then reserved = 0 end
local availableReserve = reserve - reserved
if availableReserve < 0 then availableReserve = 0 end
local reserveTaps = math.floor(availableReserve / coinPerTap)
if reserveTaps < 0 then reserveTaps = 0 end

local gained = requested
if gained > mintable then gained = mintable end
if gained > remainingDaily then gained = remainingDaily end
if gained > reserveTaps then gained = reserveTaps end
if gained < 0 then gained = 0 end

local reason = 'ok'
if gained == 0 then
  if dailyLimit ~= nil and dailyLimit > 0 and remainingDaily == 0 and mintable > 0 then
    reason = 'daily_limit'
  elseif reserveTaps == 0 and mintable > 0 and remainingDaily > 0 then
    reason = 'reserve_empty'
  elseif mintable <= 0 then
    reason = 'no_energy'
  else
    reason = 'zero'
  end
end

-- Persist regenerated energy even if gained == 0 (keeps updatedAt fresh).
energy = energy - gained
if energy < 0 then energy = 0 end

redis.call('HSET', userKey, 'energy', energy, 'energy_updated_at', now)

if dailyLimit ~= nil and dailyLimit > 0 then
  redis.call('HINCRBY', dailyKey, 'tapped', gained)
  redis.call('HSETNX', dailyKey, 'extra_quota', extraQuota)
  redis.call('EXPIRE', dailyKey, dailyTTL)
end

local coins = gained * coinPerTap
if coins > 0 then
  redis.call('HINCRBY', sysKey, 'reserve_supply', -coins)
  local id = redis.call('XADD', streamKey, 'MAXLEN', '~', streamMaxLen, '*',
    'kind', 'tap',
    'uid', userID,
    'taps', tostring(gained),
    'coins', tostring(coins),
    'day', dayStr,
    'ts', tostring(now),
    'req', tostring(requested)
  )
end

local outEnergy = math.floor(energy)
local outEnergyMax = math.floor(eMax)
local outTapped = tapped + gained
local outExtra = extraQuota
local outRemaining = 0
if dailyLimit ~= nil and dailyLimit > 0 then
  outRemaining = (dailyLimit + outExtra) - outTapped
  if outRemaining < 0 then outRemaining = 0 end
else
  outRemaining = 9223372036854775807
end

return {tostring(gained), reason, tostring(outEnergy), tostring(outEnergyMax), tostring(outTapped), tostring(outExtra), tostring(outRemaining)}
`
