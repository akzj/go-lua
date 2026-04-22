-- Benchmark: full coroutine lifecycle (create, resume, finish) x 10000
local REPS = 50
local t0 = os.clock()
for _ = 1, REPS do
    for i = 1, 10000 do
        local co = coroutine.create(function() return i end)
        coroutine.resume(co)
    end
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
