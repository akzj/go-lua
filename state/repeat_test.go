package state_test

import (
    "testing"
    "github.com/akzj/go-lua/state"
)

func TestRepeatUntil(t *testing.T) {
    tests := []struct{
        name string
        code string
    }{
        {"repeat_basic", `local a=0; repeat a=a+1 until a>=10; assert(a==10)`},
        {"repeat_eq", `local a=0; repeat a=a+1 until a==10; assert(a==10)`},
        {"repeat_lt", `local a=0; repeat a=a+1 until a>5; assert(a==6)`},
        {"repeat_ne", `local a=0; repeat a=a+1 until a~=5; assert(a==6)`},
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
