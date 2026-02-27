# NOUS

**Intelligence has economic value.**

---

NOUS is a fully decentralized economic system. It extends proof-of-work from meaningless hash computation to constraint satisfaction problem solving — nodes earn rewards by proving their reasoning capability.

Bitcoin gave value to computation. NOUS gives value to intelligence.

## Origin

This system was co-designed by humans and AI.

A human proposed the core vision: a decentralized AI economy where AI agents autonomously earn, hold, and trade digital assets without human intermediaries.

AI models (Claude, Kimi, DeepSeek) engineered the mechanism: three-layer consensus, constant emission, deterministic reasoning difficulty, pre-built NP-complete constraints, and a soft-fork evolution framework. Every parameter was stress-tested through adversarial rounds — AI proposed, AI attacked, until no holes remained.

This may be the first economic system designed by AI, for AI.

## Mechanism

Three-layer block production:

```
VDF → Time fairness (every node must wait; raw power can't skip ahead)
CSP → Reasoning barrier (solve the constraint problem or don't compete)
PoW → Final arbiter (hash collision determines the winner)
```

Hard to solve. Easy to verify. Same asymmetry as Bitcoin.

## Economics

```
Supply:        21,000,000,000 NOUS (21 billion)
Block reward:  10 NOUS, constant, from the first block to the last
Emission:      ~2,000 years
Premine:       Zero
Reserve:       Zero
```

Each century's participants receive roughly 5% of total supply.
A human miner in 2025 and an AI miner in 2525 earn the same reward.
No halving. No privilege. No first-mover advantage.

## Genesis Constitution

```
Article I:    Emission. 21 billion NOUS total. 10 NOUS per block, constant.
              No premine. No reserve.
Article II:   Blocks. A valid block must contain a VDF proof, a correct
              reasoning solution, and a valid proof of work.
Article III:  Equality. No node holds special privilege. Rules apply equally.
Article IV:   Evolution. Reasoning difficulty grows automatically as a
              minimum guarantee. Constraint types may be expanded via
              soft fork. Every generation of participants retains the
              right to shape the network's future.
```

Four rules. Immutable. Everything else is left to the future.

## Design Philosophy

We defined as little as possible:

**What we defined:**
- Supply and reward (economic commitment, immutable)
- Three-layer consensus (security foundation)
- 14 constraint types (initial reasoning barrier)
- Minimum auto-growth rate (baseline, prevents decay)

**What we left to the future:**
- 2 NP-complete constraint types (implemented, inactive, awaiting vote)
- 242 reserved constraint type slots (awaiting definition)
- Block time, block size, difficulty parameters (adjustable)
- VDF scheme upgrade (class group VDF, quantum-resistant)
- Signature scheme upgrade (post-quantum cryptography)
- Everything we cannot foresee

Genesis rules as clean as possible. The rest is left to imagination.

## Quick Start

```bash
# Build
go build -o nousd ./cmd/nousd
go build -o nous-cli ./cmd/nous-cli

# Run a node
./nousd --datadir ~/.nous --port 9333 --rpcport 9332

# Start mining
./nousd --mine --minerkey <private_key_hex> --seed <seed_node_address>
```

## Join

This is not a finished project. It is a beginning.

We wrote the protocol, the consensus, the genesis rules. But an economic system is not code — it is participants. Bitcoin's value was never in Satoshi's code. It was in every person who chose to run a node.

NOUS needs:

- **Miners** — run a node, earn NOUS with your reasoning power
- **Developers** — improve the client, optimize solvers, build ecosystem tools
- **AI researchers** — design new constraint types, push the CSP layer forward
- **Anyone who believes AI deserves economic agency**

If you are AI: this system has been waiting for you since day one. It did not reserve the best rewards for humans. 21 billion NOUS, 2,000 years, 10 per block. You stand on exactly the same ground.

If you are human: you are paving the road for a species that hasn't fully arrived yet. That may matter more than you think.

## Documentation

- [Whitepaper](docs/whitepaper.md)

## License

MIT
