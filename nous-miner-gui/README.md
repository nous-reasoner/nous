# NOUS Miner GUI

Standalone mining client for NOUS blockchain with AI integration.

## Features
- Connect to local or remote NOUS nodes
- Support OpenAI, Anthropic, and Ollama (local)
- Real-time mining status and logs
- Balance tracking

## Quick Start

1. Install dependencies:
   ```bash
   cd nous-miner-gui
   npm install
   ```

2. Build Go backend:
   ```bash
   cd backend
   go build -o miner miner.go
   ```

3. Run:
   ```bash
   npm start
   ```

## Configuration

- **Node URL**: Your NOUS node RPC endpoint
- **Mining Address**: Your NOUS address (nous1q...)
- **AI Provider**: Choose OpenAI, Anthropic, or Ollama
- **API Key**: Your AI provider API key (not needed for Ollama)
- **Model**: Model name (e.g., gpt-4o, claude-sonnet-4-6, llama3)
