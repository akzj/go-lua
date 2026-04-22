-- Benchmark: allocation + GC pressure
local REPS = 50
local t0 = os.clock()
for _ = 1, REPS do
    for i = 1, 10000 do
        local t = {i, i*2, i*3}
    end
    collectgarbage("collect")
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
