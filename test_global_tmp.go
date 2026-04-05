package main
import "github.com/akzj/go-lua/state"
func main() {
    L := state.New()
    state.DoStringOn(L, `x = 42; print("x =", x)`)
}
