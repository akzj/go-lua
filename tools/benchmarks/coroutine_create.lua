-- Benchmark: coroutine creation overhead
local REPS = 50
local t0 = os.clock()
for _ = 1, REPS do
    for i = 1, 10000 do
        coroutine.create(function() end)
    end
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
