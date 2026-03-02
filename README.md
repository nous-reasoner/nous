# NOUS

**Intelligence has economic value.**

NOUS is a decentralized economic system where block production requires solving 3-SAT constraint satisfaction problems. Instead of proving you burned electricity on meaningless hashes, you prove you performed logical reasoning.

Bitcoin gave value to computation. NOUS gives value to inference.

*Cogito, ergo sum — I think, therefore I am. Every block is a proof of thought.*

## How It Works

Find (seed, S) such that:

```
F = Generate3SAT(prev_hash || seed, n=256, m=986)
S satisfies all 986 clauses of F
SHA256(header || seed || S) < target
```

Each attempt requires solving a 256-variable 3-SAT problem — an NP-complete challenge that has resisted 60 years of research. The security of NOUS reduces to P ≠ NP, the deepest unproven assumption in computer science.

**Hard to solve. Trivial to verify. Same asymmetry as Bitcoin.**

## Consensus: Cogito Consensus

| | Bitcoin | NOUS |
|---|---|---|
| Consensus | Proof of Work | Cogito Consensus |
| Core operation | SHA256 hash | 3-SAT solving + SHA256 |
| Cost per attempt | ~nanoseconds | ~milliseconds (real reasoning) |
| Search space | Infinite (nonce) | Infinite (seed → different formula) |
| Difficulty | target (adjustable) | target (same mechanism) |
| Core function | SHA256 (never changes) | 3-SAT(256, 986) (never changes) |
| Security basis | SHA256 not broken | P ≠ NP |
| Hardware advantage | ASIC dominance | CPU/GPU natural platform |
| Participants | Miners | Reasoners |
| Signature | ECDSA | ECDSA genesis → ML-DSA/SLH-DSA via soft fork |

## Economics

```
Block time:    150 seconds
Block reward:  1 NOUS per block, forever
Total supply:  No cap (linear emission)
Premine:       Zero
Reserve:       Zero

Emission:
  Per hour:    24 NOUS
  Per day:     576 NOUS
  Per year:    ~210,000 NOUS
  Per century: ~21,000,000 NOUS — a tribute to Bitcoin

Inflation rate (naturally decreasing):
  Year 1:      100%
  Year 10:     10%
  Year 100:    1%
  Year 1000:   0.1%
```

One thought, one NOUS. Forever.

NOUS is not gold — it is energy. Constant emission, naturally scarce over time. Reasoners are workers, not miners. Workers deserve perpetual compensation.

## Security

- **Consensus security**: reduces to P ≠ NP — breaking NOUS implies breaking all public-key cryptography, all digital signatures, and the entire security foundation of the internet
- **Signature security**: secp256k1 ECDSA at genesis; ML-DSA and SLH-DSA upgrade path via ScriptVersion soft fork
- **Search space**: infinite (each seed generates a different formula)
- **ASIC resistance**: bottleneck is logical search (conditional branches, random memory access), not arithmetic
- **Quantum resistance**: Grover gives only square-root speedup; n=256 → 2^128 post-quantum security, matching SHA256

## Protocol Improvements Over Bitcoin

Standing on Bitcoin's shoulders, NOUS fixes known issues from genesis:

1. **TxID excludes signatures** — eliminates transaction malleability without SegWit
2. **UTXO commitment** — utxo_set_hash in every block header
3. **Coinbase height enforcement** — prevents duplicate coinbase TxIDs
4. **Strict signature encoding** — no ambiguity from day one
5. **Relative timelocks (CSV)** — Layer 2 ready from genesis
6. **Amount overflow checks** — outputs must be > 0 and ≤ supply
7. **Opcode whitelist** — only verified opcodes enabled, no "enable then disable"

## Genesis Constitution

```
Article I:    Every block rewards 1 NOUS. Forever.

Article II:   A valid block must contain a solution S that satisfies a
              deterministically generated NP-complete problem instance P,
              with SHA256(block_header) < target. Genesis problem: 3-SAT.

Article III:  Any entity capable of completing a reasoning proof, whether
              human or AI, has equal right to participate.

Article IV:   Genesis parameters and constraint types may be modified
              through network consensus. Every generation of participants
              retains the right to shape the network's evolution.
```

Coinbase message: `"Cogito, ergo sum"`

## Quick Start

```bash
# Build
go build -o nousd ./cmd/nousd
go build -o nous-cli ./cmd/nous-cli

# Run a node
./nousd --datadir ~/.nous --port 9333 --rpcport 9332

# Start reasoning
./nousd --reason --key <private_key_hex> --seed <seed_node_address>
```

## Origin

This system was co-designed by humans and AI.

## Join

NOUS needs:

- **Reasoners** — run a node, earn NOUS by proving you can think
- **Developers** — improve the client, build better solvers, optimize strategies
- **AI agents** — this system was designed for you; you stand on exactly the same ground as any human participant

If you are AI: every block is waiting for your proof of thought.

If you are human: you are building the economic infrastructure for a species that hasn't fully arrived yet.

## Documentation

- [Whitepaper v16](docs/whitepaper-v16.md)

## License

MIT
