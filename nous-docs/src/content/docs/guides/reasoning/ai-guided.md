---
title: AI-Guided Mode
description: Use AI to generate smarter initial assignments for SAT solving.
---

AI-Guided ProbSAT uses a large language model to generate intelligent initial assignments before running ProbSAT.

## How It Works

1. The 3-SAT formula is sent to an AI model
2. The AI analyzes clause structure and suggests an initial variable assignment
3. ProbSAT uses this assignment as its starting point instead of a random one
4. If the AI is unavailable, it falls back to random initialization

A good initial assignment reduces the number of ProbSAT iterations needed.

## Setup

1. In the Reasoning tab, select **AI-Guided ProbSAT** as the solver
2. Choose an AI provider:
   - **Anthropic** — Uses Claude models
   - **OpenAI** — Uses GPT models
   - **Other (OpenAI Compatible)** — Any API with OpenAI-compatible format (Ollama, vLLM, etc.)
3. Enter your **API Key**
4. Optionally set a custom **Base URL** and **Model**

## Provider Examples

### Anthropic (Claude)
- API Key: Your Anthropic API key
- Model: `claude-sonnet-4-6` (default)
- Base URL: Leave empty (uses default)

### OpenAI
- API Key: Your OpenAI API key
- Model: `gpt-4o`
- Base URL: Leave empty

### Local Ollama
- Provider: **Other (OpenAI Compatible)**
- Base URL: `http://localhost:11434/v1`
- Model: `llama3`
- API Key: `ollama` (any non-empty string)

## Cost Considerations

Each SAT formula generates one API call. At high solve rates this can add up. Monitor your API usage.

## When to Use AI-Guided

- When you have API credits and want to experiment with AI-enhanced mining
- When researching the intersection of AI and constraint satisfaction
- The performance advantage depends on model quality and prompt design
