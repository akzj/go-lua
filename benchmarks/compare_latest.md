# C Lua vs go-lua Performance Comparison

- **Date:** 2026-04-28 21:51:17
- **Branch:** feature/coroutine-opt @ 220525a
- **C Lua:** Lua 5.5.1  Copyright (C) 1994-2026 Lua.org, PUC-Rio
- **Runs per benchmark:** 5 (median)
- **Timing method:** `os.clock()` (CPU time, measured inside Lua)

## Results

| Benchmark | C Lua (ms) | go-lua (ms) | Ratio (go/C) |
|-----------|----------:|------------:|-------------:|
| Closure Creation                    |      33.83 |       76.05 |         2.25x |
| Concat Multi                        |       7.73 |        4.76 |         0.62x |
| Concat Operator                     |       3.94 |       31.73 |         8.05x |
| Coroutine Create                    |      44.69 |      236.78 |         5.30x |
| Coroutine Create Resume Finish      |      75.08 |      339.85 |         4.53x |
| Coroutine Yield Resume              |      37.63 |      130.56 |         3.47x |
| Fibonacci                           |      14.54 |       22.90 |         1.58x |
| For Loop                            |     119.46 |      231.34 |         1.94x |
| Gc                                  |      27.03 |       77.44 |         2.87x |
| Method Call                         |      35.51 |       56.56 |         1.59x |
| Pattern Match                       |      22.29 |       33.67 |         1.51x |
| String Concat                       |       8.39 |       28.49 |         3.40x |
| Table Ops                           |      10.94 |       21.65 |         1.98x |

| **Geometric Mean** | | | **2.48x** |

## Interpretation

- **Ratio < 2x**: Competitive with C Lua
- **Ratio 2-5x**: Acceptable for a Go implementation
- **Ratio > 5x**: Potential optimization target

## Benchmark Descriptions

| Benchmark | What it tests |
|-----------|--------------|
| Closure Creation | Closure/upvalue allocation overhead |
| Concat Multi | Multi-value string concatenation (a..b..c..d..e..f) |
| Concat Operator | Incremental string .. operator (s = s.."x" loop) |
| Coroutine Create | coroutine.create() overhead |
| Coroutine Create Resume Finish | Full coroutine lifecycle |
| Coroutine Yield Resume | yield/resume cycle throughput |
| Fibonacci | Recursive function calls + arithmetic |
| For Loop | Tight numeric for-loop (VM dispatch speed) |
| Gc | Allocation pressure + collectgarbage() |
| Method Call | Metatable method dispatch (OOP pattern) |
| Pattern Match | string.find/gsub pattern matching |
| String Concat | tostring() + table.concat() |
| Table Ops | Table creation, sequential write, sequential read |
