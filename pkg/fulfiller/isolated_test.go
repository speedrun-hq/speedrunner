package fulfiller

import (
	"math/big"
	"testing"
)

// TestBigIntMath verifies basic big.Int operations work as expected
func TestBigIntMath(t *testing.T) {
	// Simple test for big int addition
	a := big.NewInt(5)
	b := big.NewInt(10)
	c := new(big.Int).Add(a, b)

	if c.Int64() != 15 {
		t.Errorf("Expected 5 + 10 = 15, got %d", c.Int64())
	}

	// Test MaxUint256 calculation
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

	// Max uint256 should be greater than 0
	if maxUint256.Sign() <= 0 {
		t.Errorf("Expected max uint256 to be > 0, got %s", maxUint256.String())
	}
}
