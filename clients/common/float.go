package common

import "math"

// Epsilon for floating point comparisons
const Epsilon = 1e-9

// IsZero checks if a float is approximately zero
func IsZero(f float64) bool {
	return math.Abs(f) < Epsilon
}

// IsPositive checks if a float is greater than zero (with epsilon)
func IsPositive(f float64) bool {
	return f > Epsilon
}

// IsNegative checks if a float is less than zero (with epsilon)
func IsNegative(f float64) bool {
	return f < -Epsilon
}

// IsPositiveOrZero checks if a float is >= 0 (with epsilon)
func IsPositiveOrZero(f float64) bool {
	return f > -Epsilon
}

// IsNegativeOrZero checks if a float is <= 0 (with epsilon)
func IsNegativeOrZero(f float64) bool {
	return f < Epsilon
}

// Equal checks if two floats are approximately equal
func Equal(a, b float64) bool {
	return math.Abs(a-b) < Epsilon
}

// NotEqual checks if two floats are not approximately equal
func NotEqual(a, b float64) bool {
	return math.Abs(a-b) >= Epsilon
}

// GreaterThan checks if a > b (with epsilon)
func GreaterThan(a, b float64) bool {
	return a-b > Epsilon
}

// LessThan checks if a < b (with epsilon)
func LessThan(a, b float64) bool {
	return b-a > Epsilon
}

// GreaterThanOrEqual checks if a >= b (with epsilon)
func GreaterThanOrEqual(a, b float64) bool {
	return a-b > -Epsilon
}

// LessThanOrEqual checks if a <= b (with epsilon)
func LessThanOrEqual(a, b float64) bool {
	return b-a > -Epsilon
}
