# C Lua vs go-lua Performance Comparison

- **Date:** 2026-04-28 21:16:25
- **Branch:** main @ 4cece3d
- **C Lua:** Lua 5.5.1  Copyright (C) 1994-2026 Lua.org, PUC-Rio
- **Runs per benchmark:** 5 (median)
- **Timing method:** `os.clock()` (CPU time, measured inside Lua)

## Results

| Benchmark | C Lua (ms) | go-lua (ms) | Ratio (go/C) |
|-----------|----------:|------------:|-------------:|
| Closure Creation                    |      32.24 |       62.84 |         1.95x |
| Concat Multi                        |       5.25 |       11.29 |         2.15x |
| Concat Operator                     |      10.46 |       31.22 |         2.99x |
| Coroutine Create                    |      45.54 |      241.54 |         5.30x |
| Coroutine Create Resume Finish      |      72.63 |      345.22 |         4.75x |
| Coroutine Yield Resume              |      33.37 |      226.00 |         6.77x |
| Fibonacci                           |      13.77 |       26.21 |         1.90x |
| For Loop                            |     118.92 |      232.73 |         1.96x |
| Gc                                  |      24.21 |       71.91 |         2.97x |
| Method Call                         |      40.75 |       64.39 |         1.58x |
| Pattern Match                       |      25.87 |       40.76 |         1.58x |
| String Concat                       |      10.25 |       29.42 |         2.87x |
| Table Ops                           |      16.47 |       18.19 |         1.10x |

| **Geometric Mean** | | | **2.54x** |

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
