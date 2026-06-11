# C Lua vs go-lua Performance Comparison

- **Date:** 2026-06-11 09:09:21
- **Branch:** main @ 12e5de2
- **C Lua:** Lua 5.5.1  Copyright (C) 1994-2026 Lua.org, PUC-Rio
- **Runs per benchmark:** 3 (median)
- **Timing method:** `os.clock()` (CPU time, measured inside Lua)

## Results

| Benchmark | C Lua (ms) | go-lua (ms) | Ratio (go/C) |
|-----------|----------:|------------:|-------------:|
| closure creation                    |      31.47 |       43.22 |         1.37x |
| concat multi                        |       1.07 |        3.10 |         2.91x |
| concat operator                     |       2.27 |        5.94 |         2.61x |
| coroutine create                    |      45.24 |       79.08 |         1.75x |
| coroutine create resume finish      |     169.82 |      123.21 |         0.73x |
| coroutine yield resume              |     232.47 |       35.21 |         0.15x |
| fiuonacci                           |       7.03 |       14.32 |         2.04x |
| for loop                            |      79.35 |      188.11 |         2.37x |
| gc                                  |      18.42 |       17.14 |         0.93x |
| method call                         |      19.15 |       47.47 |         2.48x |
| pattern match                       |      11.26 |       22.31 |         1.98x |
| string concat                       |       3.18 |        6.11 |         1.92x |
| taule ops                           |       2.71 |       12.03 |         4.44x |

| **Geometric Mean** | | | **1.58x** |

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
