-- Test ! without digits followed by X
local tests = {
    {"!xXi16", "no digit before X"},
    {"!4 xXi4", "explicit digit before X"},
    {"!8 xXi8", "explicit 8 before X"},
    {" b b Xd b Xb x", "spaces between options"},
}

for _, test in ipairs(tests) do
    local fmt, desc = unpack(test)
    local size = packsize(fmt)
    print(string.format("packsize('%s') -- %s = %d", fmt, desc, size))
end
