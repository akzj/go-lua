-- IO Module Test
-- Tests for the 3 critical bugs:
-- 1. io.open returns (file_handle_table, nil) on success
-- 2. f:write works after opening a file
-- 3. f:read returns content string, not a function

local testFile = "/tmp/io_test_file.txt"
local allPassed = true

print("=== IO Module Tests ===")

-- Test 1: io.open return values
print("\nTest 1: io.open return values")
local f, err = io.open(testFile, "w")
print("  f type:", type(f))
print("  err:", err)
if type(f) == "table" and err == nil then
    print("  PASS: io.open returns (table, nil) on success")
else
    print("  FAIL: Expected (table, nil), got (" .. type(f) .. ", " .. tostring(err) .. ")")
    allPassed = false
end

-- Test 2: f:write works after opening
print("\nTest 2: f:write after open")
local writeOk, writeErr = f:write("hello world")
print("  write result:", writeOk, writeErr)
if writeOk ~= nil then
    print("  PASS: f:write works")
else
    print("  FAIL: f:write failed:", writeErr)
    allPassed = false
end

f:close()

-- Test 3: f:read returns content string
print("\nTest 3: f:read returns content string")
local f2, err2 = io.open(testFile, "r")
if f2 == nil then
    print("  FAIL: Could not open file for reading:", err2)
    allPassed = false
else
    local content = f2:read("*a")
    print("  content type:", type(content))
    print("  content value:", content)
    if type(content) == "string" and content == "hello world" then
        print("  PASS: f:read returns string with correct content")
    else
        print("  FAIL: Expected string 'hello world', got " .. type(content))
        allPassed = false
    end
    f2:close()
end

-- Cleanup
-- os.remove(testFile)  -- os module not available

print("\n=== Results ===")
if allPassed then
    print("ALL TESTS PASSED")
else
    print("SOME TESTS FAILED")
end