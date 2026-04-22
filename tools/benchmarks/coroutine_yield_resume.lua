-- Benchmark: yield/resume cycle overhead
local REPS = 50
local t0 = os.clock()
for _ = 1, REPS do
    local function gen()
        for i = 1, 10000 do
            coroutine.yield(i)
        end
    end
    local co = coroutine.create(gen)
    while coroutine.resume(co) do end
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
