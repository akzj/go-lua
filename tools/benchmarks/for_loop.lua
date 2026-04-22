-- Benchmark: tight numeric loop (VM dispatch speed)
local REPS = 50
local t0 = os.clock()
for _ = 1, REPS do
    local sum = 0
    for i = 1, 1000000 do
        sum = sum + i
    end
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
