-- Benchmark: closure/upvalue overhead
local REPS = 50
local t0 = os.clock()
for _ = 1, REPS do
    local function make_counter()
        local n = 0
        return function()
            n = n + 1
            return n
        end
    end
    for i = 1, 10000 do
        local c = make_counter()
        c()
        c()
    end
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
