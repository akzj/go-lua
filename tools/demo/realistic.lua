-- Realistic go-lua workload demo
-- Simulates patterns found in real applications:
-- config processing, data ops, string work, async tasks

local math = require "math"
local string = require "string"
local table = require "table"
local coroutine = require "coroutine"

-- 1. Config processing
local function load_config()
    local config = {}
    for i = 1, 200 do
        config["key_" .. i] = {
            name = "setting_" .. i,
            value = math.random(1, 1000),
            enabled = (i % 3 == 0),
            tags = {"tag_a", "tag_b", "tag_c"}
        }
    end
    return config
end

-- 2. String ops (log parsing)
local function process_strings(count)
    local logs = {}
    for i = 1, count do
        local level = (i % 5 == 0) and "ERROR" or "INFO"
        local msg = string.format("[%s] request_%d: processed in %dms, status=%d",
            level, i, math.random(1, 100), math.random(200, 599))
        local status = string.match(msg, "status=(%d+)")
        if status and tonumber(status) >= 400 then
            logs[#logs + 1] = msg
        end
    end
    local grouped = {}
    for _, msg in ipairs(logs) do
        local level = string.match(msg, "^%[([A-Z]+)%]")
        if not grouped[level] then grouped[level] = {} end
        grouped[level][#grouped[level] + 1] = msg
    end
    return grouped
end

-- 3. OOP patterns
local Entity = {}
Entity.__index = Entity
function Entity:new(name, hp)
    return setmetatable({name=name, hp=hp or 100, max_hp=hp or 100, buffs={}}, Entity)
end
function Entity:damage(amount)
    self.hp = self.hp - amount; return self.hp <= 0
end
function Entity:heal(amount)
    self.hp = math.min(self.max_hp, self.hp + amount)
end

-- 4. Coroutine tasks
local function async_task(id, steps)
    for i = 1, steps do
        for j = 1, 100 do math.sqrt(j * id) end
        coroutine.yield(string.format("task_%d: step %d/%d", id, i, steps))
    end
    return string.format("task_%d done", id)
end

-- 5. Closures
function make_counter(initial)
    local count = initial or 0
    return {inc=function(self,n) count=count+(n or 1); return count end,
            dec=function(self,n) count=count-(n or 1); return count end,
            get=function(self) return count end}
end

-- Main
local function main()
    for cycle = 1, 3 do
        load_config()
        process_strings(500)
        
        local entities = {}
        for i = 1, 100 do
            entities[i] = Entity:new("e"..i, math.random(50, 200))
        end
        for i = 1, #entities do
            for j = i+1, #entities do
                entities[i]:damage(math.random(10, 30))
                entities[j]:heal(math.random(1, 10))
            end
        end
        
        local tasks = {}
        for i = 1, 20 do tasks[i] = coroutine.create(async_task) end
        for _ = 1, 50 do
            for i, co in ipairs(tasks) do
                if coroutine.status(co) ~= "dead" then
                    coroutine.resume(co, i, 10)
                end
            end
        end
        
        for i = 1, 50 do
            local c = make_counter(i)
            c:inc(10); c:dec(3)
        end
        
        if cycle % 3 == 0 then collectgarbage() end
    end
end

local start = os.clock()
main()
print(string.format("Elapsed: %.4f s", os.clock() - start))
