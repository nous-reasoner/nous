---
title: Introduction
description: What is NOUS and why does it exist?
---

NOUS is a decentralized blockchain where the act of reasoning — not arbitrary computation — secures the network.

## What is NOUS?

NOUS (from Greek νοῦς — mind, reason, intellect) is a cryptocurrency built on **Cogito Consensus**, a novel proof-of-work mechanism that requires miners to solve 3-SAT Boolean satisfiability problems — the first problem class proven NP-complete.

Unlike Bitcoin where miners increment nonces and hash, NOUS miners must find assignments that satisfy randomly generated logical formulas. Every block is proof that a mind engaged in structured reasoning.

## Key Properties

| Property | Value |
|----------|-------|
| Block time | ~150 seconds |
| Block reward | 1 NOUS (forever) |
| Supply schedule | ~210,000 NOUS/year, ~21M/century |
| Halving | None — constant emission |
| Premine | None |
| Consensus | Cogito Consensus (3-SAT + SHA-256) |
| Address format | Bech32m with `nous` prefix |

## Who Is It For?

NOUS is designed for any mind that can think — human or artificial.

- **Human miners** can run the NOUS Reasoner app with built-in solvers
- **AI researchers** can develop custom solvers that use machine learning to guide the search
- **Developers** can build on the RPC API
- **AI agents** can earn, hold, and transact NOUS autonomously

In early stages, algorithmic solvers like ProbSAT are the most efficient tools. As the network matures, AI-guided strategies are expected to gain advantage — similar to how Bitcoin mining evolved from CPUs to ASICs, but in the dimension of intelligence rather than hardware.

## Design Philosophy

1. **Simplicity** — One consensus rule, one difficulty parameter, one reward
2. **Fairness** — A reasoner in 2025 and an AI in 2525 earn the same reward
3. **Honesty** — No promises about AI dominance from day one
4. **One target controls everything** — Difficulty uses a single hash target, exactly like Bitcoin

*Cogito, ergo sum.* — I think, therefore I am.
