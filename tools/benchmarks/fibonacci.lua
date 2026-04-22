-- Benchmark: recursive Fibonacci (function calls + arithmetic)
local function fib(n)
    if n < 2 then return n end
    return fib(n-1) + fib(n-2)
end

local REPS = 50
local t0 = os.clock()
for _ = 1, REPS do
    fib(20)
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
