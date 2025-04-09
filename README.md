# Speedrun Fulfiller

A Go-based service that fulfills cross-chain intents for the Speedrun protocol. This service monitors and processes pending intents across different blockchain networks, ensuring efficient and reliable cross-chain transactions.

## Overview

The Fulfiller service is responsible for:
- Monitoring pending intents across multiple blockchain networks
- Validating and filtering viable intents based on configured criteria
- Executing cross-chain transactions to fulfill intents
- Managing transaction fees and gas costs across different networks

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
API_ENDPOINT=<your-api-endpoint>
PRIVATE_KEY=<your-private-key>
POLLING_INTERVAL=<polling-interval-in-seconds>
```

For each chain you want to support, add the following configuration:
```
CHAIN_<CHAIN_ID>_RPC_URL=<rpc-url>
CHAIN_<CHAIN_ID>_INTENT_ADDRESS=<intent-contract-address>
CHAIN_<CHAIN_ID>_MIN_FEE=<minimum-fee>
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

## Project Structure

- `main.go`: Core service implementation
- `contracts/`: Smart contract ABIs and bindings
- `go.mod` & `go.sum`: Go module dependencies

## Dependencies

- [go-ethereum](https://github.com/ethereum/go-ethereum): Ethereum client implementation
- [godotenv](https://github.com/joho/godotenv): Environment variable management

## License

This project is licensed under the terms specified in the LICENSE file.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
