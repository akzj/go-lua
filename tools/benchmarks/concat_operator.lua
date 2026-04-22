-- Benchmark: string .. operator (VM OP_CONCAT path)
local REPS = 50
local t0 = os.clock()
for _ = 1, REPS do
    local s = ""
    for i = 1, 1000 do
        s = s .. "x"
    end
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
