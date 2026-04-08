package state_test

import (
    "testing"
    "github.com/akzj/go-lua/state"
)

func TestFloatComparison(t *testing.T) {
    tests := []struct{
        name string
        code string
    }{
        {"float_gt_int", `assert(3.14 > 3)`},
        {"float_lt_int", `assert(3.0 < 4)`},
        {"float_ge_int", `assert(3.0 >= 3)`},
        {"float_le_int", `assert(3.0 <= 3.0)`},
        {"float_ne_int", `assert(3.14 ~= 3)`},
        {"int_gt_float", `assert(4 > 3.14)`},
        {"float_eq_float", `assert(3.0 == 3.0)`},
        {"int_eq_int", `assert(3 == 3)`},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            defer func() {
                if r := recover(); r != nil {
                    t.Fatalf("panic: %v", r)
                }
            }()
            err := state.DoString(tt.code)
            if err != nil {
                t.Fatalf("DoString failed: %v", err)
            }
        })
    }
}
