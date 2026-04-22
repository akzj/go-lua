-- Benchmark: table creation, read, write
local REPS = 50
local t0 = os.clock()
for _ = 1, REPS do
    local t = {}
    for i = 1, 10000 do
        t[i] = i * 2
    end
    local sum = 0
    for i = 1, 10000 do
        sum = sum + t[i]
    end
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
