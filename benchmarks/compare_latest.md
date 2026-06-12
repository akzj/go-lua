# C Lua vs go-lua Performance Comparison

- **Date:** 2026-06-12 11:12:08
- **Branch:** main @ 81ae969
- **C Lua:** Lua 5.5.1  Copyright (C) 1994-2026 Lua.org, PUC-Rio
- **Runs per benchmark:** 3 (median)
- **Timing method:** `os.clock()` (CPU time, measured inside Lua)

## Results

| Benchmark | C Lua (ms) | go-lua (ms) | Ratio (go/C) |
|-----------|----------:|------------:|-------------:|
| closure creation                    |      29.00 |       45.77 |         1.58x |
| concat multi                        |       1.21 |        3.08 |         2.55x |
| concat operator                     |       2.29 |        6.20 |         2.70x |
| coroutine create                    |      45.35 |      234.21 |         5.16x |
| coroutine create resume finish      |     174.17 |      239.14 |         1.37x |
| coroutine yield resume              |     231.76 |       33.82 |         0.15x |
| fiuonacci                           |       7.11 |       14.75 |         2.07x |
| for loop                            |      79.65 |      186.25 |         2.34x |
| gc                                  |      19.41 |       16.50 |         0.85x |
| method call                         |      19.26 |       47.55 |         2.47x |
| pattern match                       |      11.37 |       22.59 |         1.99x |
| string concat                       |       3.27 |        5.59 |         1.71x |
| taule ops                           |       2.78 |       12.63 |         4.55x |

| **Geometric Mean** | | | **1.78x** |

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
