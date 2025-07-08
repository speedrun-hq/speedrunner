# Speedrunner

![image](logo.png)

Speedrunner is a simple service that fulfills cross-chain intents for the Speedrun protocol.
This service monitors and processes pending intents across different blockchain networks.

## Overview

The service is responsible for:
- Monitoring pending intents across supported blockchain networks
- Validating and filtering viable intents based on configured criteria
- Executing cross-chain transactions to fulfill intents
- Managing transaction fees and gas costs across different networks

## Usage

### Prerequisites

- Go 1.24 or higher
- For the networks you want to support:
  - Access to an RPC endpoint 
  - Private key of a funded hot-wallet

### Configuration

The fulfiller process can be configured using environment variables.
An example `.env.example` is provided in the repository. You can create a `.env` file based on this example.

### Running

Build the project:
```bash
go build -o speedrunner
```

Run the service:
```bash
./speedrunner
```

### Monitoring

The service exposes Prometheus metrics on the configured metrics port (default: 8080):
- `/metrics`: Prometheus metrics
- `/health`: Health check endpoint
- `/ready`: Readiness check endpoint
- `/status`: Service status details
- `/circuit/reset?chain=<chain_id>`: Reset circuit breaker for a specific chain (POST)

## Contributing

### Package Structure

The codebase is organized into the following packages:

- `pkg/blockchain`: Chain configuration and blockchain interactions
- `pkg/circuitbreaker`: Circuit breaker pattern implementation for improved reliability
- `pkg/config`: Configuration loading and validation
- `pkg/contracts`: Smart contract bindings generated from ABIs
- `pkg/fulfiller`: Core service functionality including intent processing
- `pkg/health`: Health check and metrics HTTP server
- `pkg/metrics`: Prometheus metrics for monitoring
- `pkg/models`: Data models shared across packages

### Running Tests

We provide multiple ways to run tests, with a focus on isolated tests that don't depend on external blockchain libraries.

```bash
# Run isolated tests (no ethereum dependencies)
make test-isolated

# Generate coverage report
make coverage

# Run all tests (may have dependency issues)
make test
```

#### Testing Approach

We've implemented two testing approaches:

1. **Isolated Tests**: These tests use our custom mocks in `pkg/fulfiller/mocks` instead of relying on go-ethereum libraries. They're guaranteed to run in any environment, including CI.

2. **Full Tests**: These tests include blockchain simulation and may have dependency issues with go-ethereum libraries.

Our CI pipeline runs the isolated tests to avoid dependency problems with `github.com/fjl/memsize` and other go-ethereum dependencies.

### Dependency Issues

If you encounter errors with the `github.com/fjl/memsize` dependency (e.g., `invalid reference to runtime.stopTheWorld`), you have two options:

1. Run only the isolated tests using `make test-isolated`
2. Pin a specific version of the dependency with:
   ```bash
   go mod edit -replace github.com/fjl/memsize=github.com/fjl/memsize@v0.0.0-20190710130421-bcb5799ab5e5
   go mod tidy
   ```

## License

MIT
