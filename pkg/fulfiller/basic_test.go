package fulfiller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBasic is a simple test that demonstrates table-driven testing
func TestBasic(t *testing.T) {
	// Define test cases
	tests := []struct {
		name     string
		a        int
		b        int
		expected int
	}{
		{
			name:     "Simple addition",
			a:        1,
			b:        1,
			expected: 2,
		},
		{
			name:     "Zero addition",
			a:        5,
			b:        0,
			expected: 5,
		},
		{
			name:     "Negative numbers",
			a:        -3,
			b:        5,
			expected: 2,
		},
	}

	// Run each test case as a subtest
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Perform the calculation
			result := tc.a + tc.b

			// Assert the result
			assert.Equal(t, tc.expected, result, "Addition result should match expected value")
		})
	}
}
