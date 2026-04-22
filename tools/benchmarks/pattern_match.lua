-- Benchmark: string.find/gsub pattern matching
local REPS = 10
local t0 = os.clock()
for _ = 1, REPS do
    local s = string.rep("hello world ", 100)
    for i = 1, 100 do
        string.find(s, "(%w+)%s(%w+)")
        string.gsub(s, "%w+", string.upper)
    end
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
