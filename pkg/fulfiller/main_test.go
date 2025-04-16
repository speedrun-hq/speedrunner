package fulfiller

import (
	"flag"
	"fmt"
	"log"
	"os"
	"testing"
	"time"
)

var (
	// Define flags that can be used when running tests
	verbose = flag.Bool("verbose", false, "Enable verbose test output")

	// Global test variables
	testStartTime time.Time
)

// TestMain is used to set up environment before running tests
func TestMain(m *testing.M) {
	// Parse testing flags
	flag.Parse()

	// Setup phase - runs before any tests
	fmt.Println("Setting up test environment...")
	testStartTime = time.Now()

	// Any test setup code goes here (e.g., creating test databases, mocks, etc.)
	setup()

	// Run all tests and capture exit code
	fmt.Printf("Running tests in package: github.com/speedrun-hq/speedrun-fulfiller/pkg/fulfiller\n")
	exitCode := m.Run()

	// Teardown phase - runs after all tests complete
	teardown()
	fmt.Printf("Tests completed in %v\n", time.Since(testStartTime))

	// Exit with the same code returned from tests
	os.Exit(exitCode)
}

// setup prepares the test environment
func setup() {
	if *verbose {
		log.Println("Verbose testing enabled")
	}
	// Add any setup code here (DB connections, test servers, etc.)
}

// teardown cleans up after tests
func teardown() {
	// Add cleanup code here (close connections, delete temp files, etc.)
}
