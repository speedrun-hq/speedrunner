.PHONY: test test-isolated vet lint clean setup

# Default target
all: setup test-isolated

# Set up environment
setup:
	go mod tidy

# Run isolated tests that don't depend on go-ethereum
test-isolated:
	go test -v ./pkg/fulfiller -run "TestSimple|TestIsolated|TestApproval"

# Run all tests (may have dependency issues)
test:
	go test -v ./pkg/...

# Run go vet
vet:
	go vet ./...

# Run linter
lint:
	golangci-lint run

# Generate coverage report
coverage:
	go test -v -coverprofile=coverage.out ./pkg/fulfiller -run "TestSimple|TestIsolated|TestApproval"
	go tool cover -html=coverage.out

# Clean up
clean:
	rm -f coverage.out 