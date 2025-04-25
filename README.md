# Speedrun Fulfiller

A Go-based service that fulfills cross-chain intents for the Speedrun protocol. This service monitors and processes pending intents across different blockchain networks, ensuring efficient and reliable cross-chain transactions.

## Features

- Token approval optimization with unlimited approvals
- Multi-chain support
- Multi-token support per chain (USDC, USDT, and more)
- Robust error handling and retry logic

## Overview

The Fulfiller service is responsible for:
- Monitoring pending intents across multiple blockchain networks
- Validating and filtering viable intents based on configured criteria
- Executing cross-chain transactions to fulfill intents
- Managing transaction fees and gas costs across different networks

## Package Structure

The codebase is organized into the following packages:

- `pkg/blockchain`: Chain configuration and blockchain interactions
- `pkg/circuitbreaker`: Circuit breaker pattern implementation for improved reliability
- `pkg/config`: Configuration loading and validation
- `pkg/contracts`: Smart contract bindings generated from ABIs
- `pkg/fulfiller`: Core service functionality including intent processing
- `pkg/health`: Health check and metrics HTTP server
- `pkg/metrics`: Prometheus metrics for monitoring
- `pkg/models`: Data models shared across packages

## Prerequisites

- Go 1.24 or higher
- Access to Ethereum RPC endpoints for the networks you want to support
- Private key for transaction signing
- Environment variables configuration

## Installation

1. Clone the repository:
```bash
git clone https://github.com/speedrun-hq/speedrun-fulfiller.git
cd speedrun-fulfiller
```

2. Install dependencies:
```bash
go mod download
```

## Configuration

Create a `.env` file in the root directory with the following variables:
```
# API Configuration
API_ENDPOINT=<your-api-endpoint>
POLLING_INTERVAL=<polling-interval-in-seconds>

# Wallet Configuration
PRIVATE_KEY=<your-private-key>

# Performance and Optimization Settings
WORKER_COUNT=10
METRICS_PORT=8080

# Circuit Breaker Configuration
CIRCUIT_BREAKER_ENABLED=true
CIRCUIT_BREAKER_THRESHOLD=5
CIRCUIT_BREAKER_WINDOW=5m
CIRCUIT_BREAKER_RESET=15m
```

For each chain you want to support, add the following configuration:
```
# Chain Configuration for <CHAIN_NAME>
<CHAIN_NAME>_RPC_URL=<rpc-url>
<CHAIN_NAME>_INTENT_ADDRESS=<intent-contract-address>
<CHAIN_NAME>_MIN_FEE=<minimum-fee>
<CHAIN_NAME>_GAS_MULTIPLIER=<gas-price-multiplier>

# Token addresses for <CHAIN_NAME>
<CHAIN_NAME>_USDC_ADDRESS=<usdc-token-address>
<CHAIN_NAME>_USDT_ADDRESS=<usdt-token-address>
# Add more tokens as needed
```

## Building

Build the project:
```bash
go build -o fulfiller
```

## Running

Run the fulfiller service:
```bash
./fulfiller
```

## Monitoring

The service exposes Prometheus metrics on the configured metrics port (default: 8080):
- `/metrics`: Prometheus metrics
- `/health`: Health check endpoint
- `/ready`: Readiness check endpoint
- `/status`: Service status details
- `/circuit/reset?chain=<chain_id>`: Reset circuit breaker for a specific chain (POST)

## Running Tests

We provide multiple ways to run tests, with a focus on isolated tests that don't depend on external blockchain libraries.

### Using Make (Recommended)

```bash
# Run isolated tests (no ethereum dependencies)
make test-isolated

# Generate coverage report
make coverage

# Run all tests (may have dependency issues)
make test
```

### Using Go Test Directly

```bash
# Run isolated tests only
go test -v ./pkg/fulfiller/approval_test.go ./pkg/fulfiller/simple_test.go ./pkg/fulfiller/isolated_test.go

# Run a specific test file
go test -v ./pkg/fulfiller/approval_test.go
```

## Testing Approach

We've implemented two testing approaches:

1. **Isolated Tests**: These tests use our custom mocks in `pkg/fulfiller/mocks` instead of relying on go-ethereum libraries. They're guaranteed to run in any environment, including CI.

2. **Full Tests**: These tests include blockchain simulation and may have dependency issues with go-ethereum libraries.

Our CI pipeline runs the isolated tests to avoid dependency problems with `github.com/fjl/memsize` and other go-ethereum dependencies.

## Dependency Issues

If you encounter errors with the `github.com/fjl/memsize` dependency (e.g., `invalid reference to runtime.stopTheWorld`), you have two options:

1. Run only the isolated tests using `make test-isolated`
2. Pin a specific version of the dependency with:
   ```bash
   go mod edit -replace github.com/fjl/memsize=github.com/fjl/memsize@v0.0.0-20190710130421-bcb5799ab5e5
   go mod tidy
   ```

## License

This project is licensed under the terms specified in the LICENSE file.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Multi-Token Support

The fulfiller now supports multiple tokens per chain. Currently, it supports USDC and USDT tokens on all supported chains, with the flexibility to add more in the future.

To add a new token to a chain, simply add the token address to your environment variables:

```
# Example token configuration for Ethereum
ETHEREUM_USDC_ADDRESS=0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48
ETHEREUM_USDT_ADDRESS=0xdac17f958d2ee523a2206206994597c13d831ec7

# Example token configuration for Polygon
POLYGON_USDC_ADDRESS=0x2791bca1f2de4661ed88a30c99a7a9449aa84174
POLYGON_USDT_ADDRESS=0xc2132d05d31c914a87c6611c10748aeb04b58e8f
```

Intents should specify a `token_type` field with values like "USDC" or "USDT" to indicate which token to use. If not specified, the fulfiller defaults to USDC.
