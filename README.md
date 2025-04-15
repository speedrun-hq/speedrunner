# Speedrun Fulfiller

A Go-based service that fulfills cross-chain intents for the Speedrun protocol. This service monitors and processes pending intents across different blockchain networks, ensuring efficient and reliable cross-chain transactions.

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
git clone https://github.com/speedrun-hq/fulfiller.git
cd fulfiller
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
<CHAIN_NAME>_USDC_ADDRESS=<usdc-token-address>
<CHAIN_NAME>_GAS_MULTIPLIER=<gas-price-multiplier>
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

## License

This project is licensed under the terms specified in the LICENSE file.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
