-- Benchmark: table method dispatch (OOP pattern)
local REPS = 10
local t0 = os.clock()
for _ = 1, REPS do
    local Point = {}
    Point.__index = Point
    function Point.new(x, y)
        return setmetatable({x=x, y=y}, Point)
    end
    function Point:dist()
        return (self.x^2 + self.y^2)^0.5
    end
    local p = Point.new(3, 4)
    for i = 1, 100000 do
        p:dist()
    end
end
local elapsed = os.clock() - t0
print(string.format("%.6f", elapsed))
