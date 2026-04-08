package state_test

import (
    "testing"
    "github.com/akzj/go-lua/state"
)

func TestWhileLoops(t *testing.T) {
    tests := []struct{
        name string
        code string
    }{
        {"while_basic", `local s=0; local a=1; while a<=10 do s=s+a; a=a+1 end; assert(s==55)`},
        {"while_lt", `local a=0; while a<5 do a=a+1 end; assert(a==5)`},
        {"while_false", `local a=0; while false do a=100 end; assert(a==0)`},
        {"while_nested", `local s=0; local i=1; while i<=5 do local j=1; while j<=i do s=s+1; j=j+1 end; i=i+1 end; assert(s==15)`},
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
