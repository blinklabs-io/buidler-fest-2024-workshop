# buidler-fest-2024-workshop

A Cardano blockchain indexer and reward system that monitors transactions and automatically sends rewards based on configurable criteria.

## Environment Variables

The application uses environment variables to configure its behavior. You can set these variables in your environment or create a `.env` file in the project root.

### General
- `NETWORK`: Cardano network to use (default: `preprod`)

### Indexer
Use one of the following:
- `INDEXER_TCP_ADDRESS`: TCP address and port of the remote Cardano Node for the indexer
- `INDEXER_SOCKET_PATH`: Socket path of the local Cardano Node for the indexer

### Reward
- `MIN_LOVELACE`: Minimum Lovelace required to trigger a reward (default: `50_000_000`)
- `REWARD_ADDRESS`: Address to send rewards to
- `REWARD_AMOUNT`: Amount of Lovelace to send as a reward (default: `5_000_000`)
- `SOURCE_ADDRESS`: Source address to filter transactions

### Submit
Use one of the following:
- `SUBMIT_TCP_ADDRESS`: TCP address and port of the remote Cardano Node for transaction submission
- `SUBMIT_SOCKET_PATH`: Socket path of the local Cardano Node for transaction submission
- `SUBMIT_URL`: API URL for transaction submission

### TxBuilder
Use one of the following:
- `BLOCKFROST_API_KEY`: Blockfrost API key for UTxO queries
- `KUPO_URL`: Kupo URL for UTxO queries

### Wallet
- `MNEMONIC`: Wallet mnemonic (if not set, will use or generate `seed.txt`)

## Application Workflow

### 1. Startup (`cmd/workshop/main.go`)
- The main entry point is the `main()` function
- Loads configuration from environment variables and `.env` file
- Sets up logging
- Initializes the wallet (loads or generates mnemonic)
- Starts the indexer

### 2. Wallet Setup (`internal/wallet/wallet.go`)
- Loads mnemonic from config or `seed.txt`
- If not present, generates a new mnemonic and writes it to `seed.txt`
- Initializes the wallet for use

### 3. Indexer (`internal/indexer/indexer.go`)
- Creates a pipeline to listen for transaction events on the configured network and addresses
- Filters events for relevant addresses (wallet and reward)
- On transaction events, triggers the transaction builder

### 4. Transaction Builder (`internal/txbuilder/txbuilder.go`)
- Handles transaction events
- Checks if the transaction meets reward criteria (source address, minimum Lovelace, etc.)
- Builds a reward transaction if criteria are met
- Signs the transaction with the wallet keys

### 5. Transaction Submission (`internal/txsubmit/txsubmit.go`)
- Submits the built transaction to the network via TCP, socket, or API, depending on config

## Building and Running

To build the application:

```bash
make build
```

To run the application:

```bash
./workshop
```

The application will run continuously, monitoring the blockchain for transactions and automatically sending rewards when criteria are met.