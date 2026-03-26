// Package api provides the public Lua API
// This file tests the math standard library module
package api

import (
	"math"
	"testing"
)

// TestMathAbs tests math.abs function
func TestMathAbs(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"positive number", "return math.abs(42)", 42.0},
		{"negative number", "return math.abs(-42)", 42.0},
		{"zero", "return math.abs(0)", 0.0},
		{"negative zero", "return math.abs(-0)", 0.0},
		{"small decimal", "return math.abs(-3.14)", 3.14},
		{"large number", "return math.abs(-1000000)", 1000000.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathCeil tests math.ceil function
func TestMathCeil(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"integer", "return math.ceil(5.0)", 5.0},
		{"positive decimal", "return math.ceil(5.3)", 6.0},
		{"negative decimal", "return math.ceil(-5.3)", -5.0},
		{"zero", "return math.ceil(0)", 0.0},
		{"small positive", "return math.ceil(0.1)", 1.0},
		{"small negative", "return math.ceil(-0.1)", 0.0},
		{"large number", "return math.ceil(999999.1)", 1000000.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathFloor tests math.floor function
func TestMathFloor(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"integer", "return math.floor(5.0)", 5.0},
		{"positive decimal", "return math.floor(5.9)", 5.0},
		{"negative decimal", "return math.floor(-5.1)", -6.0},
		{"zero", "return math.floor(0)", 0.0},
		{"small positive", "return math.floor(0.9)", 0.0},
		{"small negative", "return math.floor(-0.1)", -1.0},
		{"large number", "return math.floor(999999.9)", 999999.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathMax tests math.max function
func TestMathMax(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"two positives", "return math.max(1, 2)", 2.0},
		{"two negatives", "return math.max(-1, -2)", -1.0},
		{"mixed signs", "return math.max(-5, 5)", 5.0},
		{"three numbers", "return math.max(1, 3, 2)", 3.0},
		{"four numbers", "return math.max(10, 5, 20, 15)", 20.0},
		{"single argument", "return math.max(42)", 42.0},
		{"zero included", "return math.max(-1, 0, 1)", 1.0},
		{"large numbers", "return math.max(1000000, 2000000)", 2000000.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathMin tests math.min function
func TestMathMin(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"two positives", "return math.min(1, 2)", 1.0},
		{"two negatives", "return math.min(-1, -2)", -2.0},
		{"mixed signs", "return math.min(-5, 5)", -5.0},
		{"three numbers", "return math.min(1, 3, 2)", 1.0},
		{"four numbers", "return math.min(10, 5, 20, 15)", 5.0},
		{"single argument", "return math.min(42)", 42.0},
		{"zero included", "return math.min(-1, 0, 1)", -1.0},
		{"large numbers", "return math.min(1000000, 2000000)", 1000000.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathSqrt tests math.sqrt function
func TestMathSqrt(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"perfect square", "return math.sqrt(16)", 4.0},
		{"one", "return math.sqrt(1)", 1.0},
		{"zero", "return math.sqrt(0)", 0.0},
		{"two", "return math.sqrt(2)", math.Sqrt(2)},
		{"large number", "return math.sqrt(1000000)", 1000.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathSqrtNegative tests that sqrt of negative returns NaN
func TestMathSqrtNegative(t *testing.T) {
	result := doStringNumber(t, "return math.sqrt(-1)")
	if !math.IsNaN(result) {
		t.Errorf("Expected NaN for sqrt(-1), got %f", result)
	}
}

// TestMathPow tests math.pow function
func TestMathPow(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"power of two", "return math.pow(2, 3)", 8.0},
		{"power of zero", "return math.pow(5, 0)", 1.0},
		{"power of one", "return math.pow(7, 1)", 7.0},
		{"zero base", "return math.pow(0, 5)", 0.0},
		{"negative exponent", "return math.pow(2, -1)", 0.5},
		{"fractional exponent", "return math.pow(4, 0.5)", 2.0},
		{"negative base even power", "return math.pow(-2, 2)", 4.0},
		{"negative base odd power", "return math.pow(-2, 3)", -8.0},
		{"large exponent", "return math.pow(10, 6)", 1000000.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathSin tests math.sin function
func TestMathSin(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"sin(0)", "return math.sin(0)", 0.0},
		{"sin(π/2)", "return math.sin(math.pi / 2)", 1.0},
		{"sin(π)", "return math.sin(math.pi)", 0.0}, // approximately 0
		{"sin(-π/2)", "return math.sin(-math.pi / 2)", -1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			// Use approximate comparison for floating point
			if math.Abs(result-tt.expected) > 1e-10 {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathCos tests math.cos function
func TestMathCos(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"cos(0)", "return math.cos(0)", 1.0},
		{"cos(π/2)", "return math.cos(math.pi / 2)", 0.0}, // approximately 0
		{"cos(π)", "return math.cos(math.pi)", -1.0},
		{"cos(-π)", "return math.cos(-math.pi)", -1.0},
		{"cos(2π)", "return math.cos(2 * math.pi)", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			// Use approximate comparison for floating point
			if math.Abs(result-tt.expected) > 1e-10 {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathTan tests math.tan function
func TestMathTan(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"tan(0)", "return math.tan(0)", 0.0},
		{"tan(π/4)", "return math.tan(math.pi / 4)", 1.0},
		{"tan(-π/4)", "return math.tan(-math.pi / 4)", -1.0},
		{"tan(π)", "return math.tan(math.pi)", 0.0}, // approximately 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			// Use approximate comparison for floating point
			if math.Abs(result-tt.expected) > 1e-10 {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathPi tests math.pi constant
func TestMathPi(t *testing.T) {
	result := doStringNumber(t, "return math.pi")
	if result != math.Pi {
		t.Errorf("Expected %f, got %f", math.Pi, result)
	}
}

// TestMathHuge tests math.huge constant
func TestMathHuge(t *testing.T) {
	result := doStringNumber(t, "return math.huge")
	if !math.IsInf(result, 1) {
		t.Errorf("Expected +Inf for math.huge, got %f", result)
	}
}

// TestMathRandomNoArgs tests math.random() with no arguments
func TestMathRandomNoArgs(t *testing.T) {
	// Run multiple times to ensure it's in valid range
	for i := 0; i < 100; i++ {
		result := doStringNumber(t, "return math.random()")
		// Result should be in [0, 1)
		if result < 0 || result >= 1 {
			t.Errorf("math.random() returned %f, expected [0, 1)", result)
		}
	}
}

// TestMathRandomOneArg tests math.random(n) with one argument
func TestMathRandomOneArg(t *testing.T) {
	// Test with n=10
	for i := 0; i < 100; i++ {
		result := doStringNumber(t, "return math.random(10)")
		// Result should be in [1, 10]
		if result < 1 || result > 10 {
			t.Errorf("math.random(10) returned %f, expected [1, 10]", result)
		}
		// Should be integer
		if result != math.Floor(result) {
			t.Errorf("math.random(10) returned non-integer: %f", result)
		}
	}
}

// TestMathRandomTwoArgs tests math.random(m, n) with two arguments
func TestMathRandomTwoArgs(t *testing.T) {
	// Test with m=5, n=10
	for i := 0; i < 100; i++ {
		result := doStringNumber(t, "return math.random(5, 10)")
		// Result should be in [5, 10]
		if result < 5 || result > 10 {
			t.Errorf("math.random(5, 10) returned %f, expected [5, 10]", result)
		}
		// Should be integer
		if result != math.Floor(result) {
			t.Errorf("math.random(5, 10) returned non-integer: %f", result)
		}
	}
}

// TestMathRandomSameArgs tests math.random(n, n) returns n
func TestMathRandomSameArgs(t *testing.T) {
	for i := 0; i < 10; i++ {
		result := doStringNumber(t, "return math.random(7, 7)")
		if result != 7 {
			t.Errorf("math.random(7, 7) returned %f, expected 7", result)
		}
	}
}

// TestMathRandomLargeRange tests math.random with large range
func TestMathRandomLargeRange(t *testing.T) {
	for i := 0; i < 10; i++ {
		result := doStringNumber(t, "return math.random(1, 1000000)")
		// Result should be in [1, 1000000]
		if result < 1 || result > 1000000 {
			t.Errorf("math.random(1, 1000000) returned %f, expected [1, 1000000]", result)
		}
	}
}

// TestMathInvalidArguments tests math functions with invalid arguments
func TestMathInvalidArguments(t *testing.T) {
	// Test that invalid arguments return 0 (as per implementation)
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"abs with nil", "return math.abs(nil)", 0.0},
		{"ceil with nil", "return math.ceil(nil)", 0.0},
		{"floor with nil", "return math.floor(nil)", 0.0},
		{"sqrt with nil", "return math.sqrt(nil)", 0.0},
		{"sin with nil", "return math.sin(nil)", 0.0},
		{"cos with nil", "return math.cos(nil)", 0.0},
		{"tan with nil", "return math.tan(nil)", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathMaxEmpty tests math.max with no arguments
func TestMathMaxEmpty(t *testing.T) {
	result := doStringNumber(t, "return math.max()")
	if result != 0 {
		t.Errorf("Expected 0 for math.max() with no args, got %f", result)
	}
}

// TestMathMinEmpty tests math.min with no arguments
func TestMathMinEmpty(t *testing.T) {
	result := doStringNumber(t, "return math.min()")
	if result != 0 {
		t.Errorf("Expected 0 for math.min() with no args, got %f", result)
	}
}

// TestMathCombined tests combined math operations
func TestMathCombined(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{"abs of negative sqrt", "return math.abs(math.sqrt(16))", 4.0},
		{"floor of power", "return math.floor(math.pow(2, 3.5))", 11.0},
		{"max of abs values", "return math.max(math.abs(-10), math.abs(-5))", 10.0},
		{"sin squared plus cos squared", "local x = 1.5; return math.sin(x)^2 + math.cos(x)^2", 1.0},
		{"ceil of sqrt", "return math.ceil(math.sqrt(2))", 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			// Use approximate comparison for floating point
			if math.Abs(result-tt.expected) > 1e-10 {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestMathEdgeCases tests edge cases for math functions
func TestMathEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		checkResult func(float64) bool
		description string
	}{
		{"sqrt of very small number", "return math.sqrt(0.0001)", func(r float64) bool { return r > 0 && r < 0.1 }, "should be small positive"},
		{"pow(0, 0)", "return math.pow(0, 0)", func(r float64) bool { return r == 1 }, "should be 1"},
		{"abs of huge", "return math.abs(-math.huge)", func(r float64) bool { return math.IsInf(r, 1) }, "should be +Inf"},
		{"max with huge", "return math.max(1, math.huge)", func(r float64) bool { return math.IsInf(r, 1) }, "should be +Inf"},
		{"min with huge", "return math.min(1, math.huge)", func(r float64) bool { return r == 1 }, "should be 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			if !tt.checkResult(result) {
				t.Errorf("%s: got %f", tt.description, result)
			}
		})
	}
}

// TestMathTrigIdentities tests trigonometric identities
func TestMathTrigIdentities(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		expected    float64
		tolerance   float64
		description string
	}{
		{"sin^2 + cos^2 = 1", "return math.sin(1)^2 + math.cos(1)^2", 1.0, 1e-10, "Pythagorean identity"},
		{"tan = sin/cos", "return math.tan(1) - math.sin(1)/math.cos(1)", 0.0, 1e-10, "tan identity"},
		{"sin(π) ≈ 0", "return math.sin(math.pi)", 0.0, 1e-10, "sin(π) should be ~0"},
		{"cos(π) = -1", "return math.cos(math.pi)", -1.0, 1e-10, "cos(π) should be -1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			if math.Abs(result-tt.expected) > tt.tolerance {
				t.Errorf("%s: expected %f, got %f (tolerance %f)", tt.description, tt.expected, result, tt.tolerance)
			}
		})
	}
}

// TestMathFloorCeilSymmetry tests that floor and ceil are symmetrical
func TestMathFloorCeilSymmetry(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"floor(ceil(x)) for positive", "return math.floor(math.ceil(5.3))"},
		{"ceil(floor(x)) for positive", "return math.ceil(math.floor(5.3))"},
		{"floor(ceil(x)) for negative", "return math.floor(math.ceil(-5.3))"},
		{"ceil(floor(x)) for negative", "return math.ceil(math.floor(-5.3))"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doStringNumber(t, tt.code)
			// These should all be integers
			if result != math.Floor(result) {
				t.Errorf("Expected integer result, got %f", result)
			}
		})
	}

}
