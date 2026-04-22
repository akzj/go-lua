-- Benchmark: string operations (build strings, table.concat)
local REPS = 50
local t0 = os.clock()
for _ = 1, REPS do
    local t = {}
    for i = 1, 1000 do
        t[i] = tostring(i)
    end
    local s = table.concat(t, ",")
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
