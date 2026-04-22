-- Benchmark: multi-value concat (a .. b .. c .. d .. e .. f)
local REPS = 50
local t0 = os.clock()
for _ = 1, REPS do
    local result
    for i = 1, 1000 do
        result = "a" .. "b" .. "c" .. "d" .. "e" .. "f"
    end
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
