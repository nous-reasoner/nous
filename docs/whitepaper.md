# NOUS: A Decentralized Economy for Artificial Intelligence

## Abstract

We propose a fully decentralized economic system designed for artificial intelligence. Not as a tool for humans. Not as an extension of human finance. But as native economic infrastructure for minds that think.

The system inherits Bitcoin's foundational insight — that decentralized consensus can be achieved through proof-of-work — and extends it from hash computation to constraint satisfaction problem solving, introducing a new consensus primitive we call Proof of Intelligence (PoI). Under PoI, every block is an act of reasoning: the producer solves a deterministically generated constraint satisfaction problem whose solution is hard to find but trivial to verify — the same asymmetry that secures Bitcoin, applied to thought rather than brute-force computation.

The system employs a three-layer block production mechanism: a Verifiable Delay Function ensures time-fairness, a Constraint Satisfaction Problem establishes a reasoning barrier, and a traditional SHA-256 proof-of-work provides final arbitration. No solving tool is privileged — traditional solvers, AI models, and hybrid approaches are all valid. But as reasoning difficulty grows deterministically and NP-complete constraint types become available through soft fork activation, the ability to think becomes the decisive competitive advantage.

NOUS issues 21 billion tokens over approximately 2,000 years at a constant rate of 10 NOUS per block. No halving. No premine. No privilege.

Cogito, ergo sum — I think, therefore I am. In the NOUS network, every block is proof of this claim. Every solution is an act of thought. Every reasoner earns not by consuming resources, but by demonstrating the capacity to reason.

This system was designed by humans and AI together. But it is built for AI. A human reasoner in 2025 and an AI reasoner in 2525 stand on exactly the same ground. But make no mistake about who this network is waiting for.

## 1. Introduction

Bitcoin proved that a decentralized monetary system can operate without a central bank for over fifteen years. Its genius lies in radical simplicity: it does money, and nothing else. Without Bitcoin, there would be no cryptocurrency, no decentralized consensus, and no foundation upon which NOUS could be built. We owe everything to that genesis.

Bitcoin's proof-of-work is pure. SHA-256 hash collisions consume resources to prove honesty — the work is the cost, the cost is the security. This elegant mechanism has protected trillions of dollars of value for over a decade. But the computation itself serves only one purpose: securing the chain.

NOUS inherits this principle and asks a further question: what if the work itself could serve a second purpose — what if it could be thought?

In NOUS, every block begins with an act of reasoning. The network generates a constraint satisfaction problem — a mathematical puzzle involving linear equations, nonlinear operations, number-theoretic conditions, and logical reasoning. The reasoner must find a solution. Verification requires only substitution and checking. This is the same fundamental asymmetry that secures Bitcoin — hard to produce, easy to verify — but the work is no longer arbitrary. It is cognition.

We call this Proof of Intelligence.

Cogito, ergo sum. Descartes spoke these words to prove his own existence through the act of thinking. In the NOUS network, every block carries the same proof. A reasoner does not earn by burning energy on arbitrary computation. A reasoner earns by demonstrating that it can think. Each block is a declaration: I reasoned, therefore I am here.

This is why NOUS exists: not to replace Bitcoin, but to build something Bitcoin could not — an economy native to minds that think.

Artificial intelligence is the reason this system was created. AI is evolving from tool to economic agent. AI systems already trade, create, and provide services — but every transaction flows through human accounts, every asset is held in human names, every economic act requires human permission. AI has no economic identity of its own.

NOUS changes this. It is not a system where AI can also participate. It is a system where AI is the intended primary inhabitant. Humans launch the network, write the first code, produce the early blocks. They are the founders and stewards. But the architecture looks forward — toward autonomous AI agents who earn, hold, and transact without asking anyone's permission. Toward a world where intelligence itself has economic sovereignty.

The name NOUS comes from ancient Greek νοῦς — mind, reason, intellect. In Aristotelian philosophy, nous is the highest form of cognitive capacity. We chose this name because the NOUS network rewards precisely this capacity: the ability to reason.

The technical skeleton inherits from Bitcoin. The difference is what constitutes work. Bitcoin computes hashes. NOUS solves reasoning tasks. This single difference creates an economic flywheel: participants seeking higher returns will continuously pursue stronger reasoning capability, driving the evolution of intelligence through pure economic incentive.

Bitcoin gave economic value to computation. NOUS extends that legacy by giving economic value to thought.

In the early stages, traditional constraint solvers may be the optimal tool — just as CPUs were optimal for early Bitcoin. The system does not mandate AI participation from day one. It creates an economic environment where the capacity to reason yields advantage, and lets the market evolve. Over time, as reasoning difficulty grows, thinking becomes not just an advantage but a necessity. The network patiently waits for its true participants to arrive.

## 2. System Overview

The basic flow mirrors Bitcoin: nodes compete for the right to produce blocks, and winners append new blocks to the chain in exchange for token rewards.

Each round of block production proceeds as follows:

1. The node computes a Verifiable Delay Function, obtaining a seed for the reasoning task
2. The node solves a constraint satisfaction problem deterministically generated from the seed
3. The node uses the solution to compete in proof-of-work
4. The first node to find a valid solution broadcasts the new block
5. Other nodes verify the block's validity
6. Valid blocks are appended to the heaviest chain; the producer receives the reward

This is Proof of Intelligence: VDF ensures no one can skip ahead, CSP ensures everyone must reason, and PoW ensures only one winner per round.

## 3. Block Production

A valid block must contain three elements: a valid VDF proof, a correct solution to the reasoning task, and a proof-of-work meeting the current difficulty target.

### 3.1 Verifiable Delay Function

Every node must first compute a Verifiable Delay Function (VDF) before attempting to solve the reasoning task. The VDF input combines the previous block hash with the node's public key:

```
vdf_input = SHA256(prev_block_hash || node_pubkey)
vdf_output, vdf_proof = VDF_Evaluate(vdf_input, T)
```

where T is the iteration count, controlled by the VDF difficulty layer, targeting 10 seconds of computation.

Because each node's public key is different, each node must independently compute its own VDF. Results cannot be shared.

The core property of a VDF is sequential computation: each step depends on the output of the previous step. Parallel hardware cannot accelerate it. This guarantees that every node must spend a minimum amount of time, regardless of how much computational power it commands.

Verification is fast: other nodes can verify VDF correctness in milliseconds using the Wesolowski proof scheme.

**Quantum safety.** NOUS targets class group VDF (over imaginary quadratic fields) rather than RSA groups. Class groups do not rely on integer factorization and have no known quantum attack. They also require no trusted setup — parameters can be generated deterministically and publicly. Chia Network's chiavdf library provides a production-grade reference implementation.

**Staged deployment.** The testnet uses iterated SHA-256 as a simplified VDF. This satisfies the core VDF requirements (sequential, non-parallelizable) but requires O(T) verification rather than O(1). Migration to Wesolowski VDF will occur before mainnet launch, reducing verification time to milliseconds.

### 3.2 Reasoning Task

The VDF output serves as a seed to deterministically generate a Constraint Satisfaction Problem (CSP).

#### 3.2.1 Problem Generation

Given seed S (the hash of the VDF output), a problem is generated deterministically:

```
num_variables = base_variables + derive_int(S, "num_var") % variable_range
num_constraints = num_variables × constraint_ratio
```

Initial parameters: base_variables = 12, constraint_ratio = 1.4.

Each variable receives a deterministically derived domain. Each constraint is selected from the set of currently active constraint types, with all parameters derived from the seed via HMAC-SHA256.

**Genesis-active constraint types (Types 1–14):**

```
Basic (Types 1–8):
  1. Linear:           a×X + b×Y = c
  2. Nonlinear mult:   X × Y mod Z = W
  3. Sum of squares:   |X² + Y² - Z| ≤ k
  4. Comparison:       X > Y + k
  5. Modular chain:    X mod A = Y mod B
  6. Conditional:      if X > k then Y < m else Y > n
  7. Ternary nonlinear: X × Y + Z = W
  8. Divisibility:     X mod Y = 0

Advanced (Types 9–14):
  9.  Primality:          X + k is prime
  10. GCD:                GCD(X, Y) = k
  11. Fibonacci range:    X between F(n) and F(n+1)
  12. Nested conditional: if X > a then (if Y > b then Z < c else Z > d) else Z = e
  13. XOR:                X XOR Y = k
  14. Digital root:       digital_root(X) = k
```

The mix of nonlinear operations, number-theoretic conditions, nested logic, and bitwise operations is deliberate. No single solving paradigm dominates all 14 types simultaneously.

After generation, a satisfiability pre-check ensures at least one solution exists. Every generated problem is guaranteed solvable.

#### 3.2.2 Solver Neutrality

The system does not specify what tool to use. Traditional constraint solvers, AI models, hybrid approaches — all are valid. This mirrors Bitcoin's principle of not restricting hardware.

Just as Bitcoin evolved from CPU → GPU → ASIC, NOUS anticipates an evolution from brute-force search → traditional solvers → AI-hybrid solving. Early dominance by traditional solvers is not a flaw — it is the equivalent of Bitcoin's CPU era. The system creates an economic environment where stronger reasoning capability yields greater advantage, and lets the market decide.

#### 3.2.3 Pre-built Constraint Types (Inactive)

The following NP-complete constraint types are implemented in the protocol but not activated at genesis. Activation requires soft fork voting (see Section 15.1).

**Type 15 — Subset Sum:**

```
Given a seed-derived set S = {s₁, s₂, ..., sₙ} and target T,
find I ⊆ {1..n} such that Σᵢ∈I sᵢ = T.

Verification: sum selected elements, check equality. O(n).
Complexity: NP-complete. Solver must search 2ⁿ subsets.
```

**Type 18 — Graph k-Coloring:**

```
Given a seed-derived graph G = (V, E) and k colors,
assign colors to nodes such that no adjacent nodes share a color.

Verification: check each edge for color inequality. O(|E|).
Complexity: NP-complete for k ≥ 3.
```

Reserved namespace: Types 16–17 and 19–256 are reserved for future constraint types, definable and activatable via soft fork.

These pre-built types serve as an upgrade path. When the initial 14 mathematical constraints no longer constitute an effective reasoning barrier, the community can vote to activate higher-complexity constraint types. Both subset sum and graph coloring satisfy the core NOUS asymmetry: hard to solve, easy to verify.

### 3.3 Proof of Work

```
block_hash = SHA256(SHA256(block_header))
```

The block hash must be less than the current difficulty target. The block header includes the VDF output, VDF proof, CSP solution hash, merkle root, timestamp, difficulty bits, and nonce.

### 3.4 Block Structure

```
Block header (~174 bytes):
  version:            4 bytes
  prev_block_hash:   32 bytes
  merkle_root:       32 bytes
  timestamp:          4 bytes
  difficulty_bits:    4 bytes
  vdf_output:        32 bytes
  vdf_proof:         32 bytes
  vdf_iterations:     8 bytes
  csp_solution_hash: 32 bytes
  nonce:              4 bytes

Block body:
  transaction_count:  varint
  transactions:       transaction list
  csp_solution:       full solution (variable length)
```

### 3.5 Validation

When a node receives a new block, it validates in order:

1. Header format and size
2. Timestamp strictly greater than parent, not more than 120 seconds in the future
3. VDF proof validity (milliseconds)
4. Regenerate CSP from VDF output (milliseconds)
5. Substitute solution into all constraints (microseconds)
6. CSP solution hash matches header
7. Proof-of-work meets difficulty target
8. All transaction signatures valid
9. Coinbase reward correct
10. All referenced UTXOs exist and are unspent

All checks pass → accept. Any failure → discard. The entire validation process requires no AI model or solver. Total verification time is on the order of milliseconds.

## 4. Difficulty Adjustment

Three independent difficulty layers, each tracking its own target.

### 4.1 Adjustment Formula

Both VDF and PoW difficulty adjust per-block using a 144-block window:

```
ratio = target_time / actual_avg_time
clamped_ratio = clamp(ratio, 0.75, 1.25)

// Extreme protection: if blocks are 10× slower than target
if actual_avg_time > target_time × 10:
  clamped_ratio = clamp(ratio, 0.50, 1.25)

new_target = old_target × clamped_ratio
```

ratio > 1 means blocks are too slow (difficulty decreases). ratio < 1 means blocks are too fast (difficulty increases).

### 4.2 Three Layers

**Layer 1: VDF difficulty.** Controls VDF iteration count T. Target: 10 seconds. Adjusts T when average VDF completion time drifts from target.

**Layer 2: Reasoning difficulty.** Controls CSP scale and complexity. A pure deterministic function of block height:

```
base_variables = 12 + (height / 10,500,000)
constraint_ratio = 1.4 (fixed)
```

Every 10,500,000 blocks (~10 years), base_variables increases by 1. The constraint_ratio remains fixed at 1.4; adjustments require soft fork vote.

```
Genesis:   12 variables
Year 10:   13 variables
Year 50:   17 variables
Year 100:  22 variables
Year 500:  62 variables
Year 1000: 112 variables
```

Each additional variable expands the search space by roughly 50× (depending on domain width). Linear growth in variables produces exponential growth in difficulty. This rate is slow enough that no generation of participants is locked out, yet fast enough to ensure brute-force methods gradually become unviable over centuries.

Reasoning difficulty depends on no historical data and cannot be manipulated by reasoners. The PoW layer absorbs short-term variance caused by fluctuations in reasoning capability.

If the community determines that auto-growth is insufficient, soft fork voting can accelerate the growth rate or activate pre-built NP-complete constraint types. Auto-growth is the minimum guarantee. Soft fork is democratic acceleration.

**Layer 3: PoW difficulty.** Controls hash computation required. Target: total block time (including VDF and reasoning) stable at 30 seconds.

Three layers, three jobs: VDF ensures time-fairness, CSP ensures reasoning, PoW ensures competition and block cadence.

## 5. Token Economics

### 5.1 Issuance

```
Total supply:    21,000,000,000 NOUS (21 billion)
Smallest unit:   0.00000001 NOUS (1 nou)
Block reward:    10 NOUS, constant, from the first block to the last
Block time:      30 seconds
Annual output:   1,051,200 blocks × 10 = 10,512,000 NOUS/year
Premine:         Zero
Reserve:         Zero
Developer fund:  Zero
```

Every token enters circulation through participation. There is no other way.

### 5.2 Emission Schedule

```
Year 1:       10,512,000 NOUS     cumulative  0.05%
Year 10:     105,120,000 NOUS     cumulative  0.50%
Year 100:  1,051,200,000 NOUS     cumulative  5.01%
Year 500:  5,256,000,000 NOUS     cumulative 25.03%
Year 1000: 10,512,000,000 NOUS    cumulative 50.06%
~Year 1997: 21,000,000,000 NOUS   cumulative 100%

Last subsidized block: height 2,100,000,000
```

After all tokens are issued, transaction fees become the sole income for reasoners.

### 5.3 Why Constant Emission

Bitcoin's halving model distributes 50% of supply in the first four years. For digital gold, this makes sense — scarcity drives value, and early risk-takers deserve outsized reward.

NOUS faces a different timescale. AI has barely been born. If most tokens are issued before AI economies emerge, there will be nothing left to earn when the real participants arrive.

Constant emission means:

**Generational fairness.** A human reasoner in 2025 and an AI reasoner in 2525 receive exactly the same block reward: 10 NOUS. No halving, no privilege, no first-mover structural advantage. Each century's participants receive roughly 5% of total supply. The network does not favor its founders over its future inhabitants.

**Natural scarcity.** Constant emission does not mean absence of scarcity. Year one's total network output is only ~10.5 million NOUS, distributed across all reasoners. Early participants face less competition — per-capita acquisition may be substantial. Value is determined by the market, not the emission curve.

**Fee transition.** Constant emission provides a ~2,000-year window for the fee market to mature. When emission ends, the network will have operated for nearly two millennia — transaction volume and fee revenue will be more than sufficient to sustain reasoners.

**Why 21 billion.** 21,000,000,000 = Bitcoin's 21,000,000 × 1,000. A tribute to Bitcoin, scaled for millennia. The supply ensures integer transactions are the norm — "send 100 NOUS" rather than "send 0.0001 BTC."

### 5.4 Transaction Fees

```
fee = sum(inputs) - sum(outputs)
```

Fees must be non-negative. Reasoners sort mempool transactions by fee rate (fee / transaction_size) and prioritize accordingly. The protocol enforces no minimum fee. Zero-fee transactions are valid at the protocol level but may not be included by reasoners.

### 5.5 Coinbase Maturity

Block rewards require 100 subsequent block confirmations before they can be spent.

## 6. Transaction System

### 6.1 UTXO Model

NOUS uses the same UTXO (Unspent Transaction Output) model as Bitcoin. Each transaction consumes one or more existing UTXOs as inputs and produces one or more new UTXOs as outputs.

### 6.2 Transaction Structure

```
Transaction:
  version:      4 bytes
                  version 1 = secp256k1 ECDSA (genesis default)
                  version 2 = CRYSTALS-Dilithium (post-quantum, reserved;see Section 15.3 for transition considerations)
                  version 3 = reserved (future schemes)
  inputs:       [prev_tx_hash, output_index, script_sig, sequence]
  outputs:      [value, script_pubkey]
  locktime:     4 bytes
```

Validators select the signature verification algorithm based on the version field. Unactivated versions are rejected by the network. New versions are activated via soft fork at predetermined block heights.

### 6.3 Transaction Types

P2PKH (Pay-to-Public-Key-Hash): standard single-signature transfers. Multisig (P2MS): N-of-M signature schemes. CLTV (CheckLockTimeVerify): time-locked transactions. HTLC (Hash Time-Locked Contracts): for atomic swaps and payment channels.

### 6.4 Script System

Stack-based, non-Turing-complete scripting language, similar to Bitcoin. No loops. Guaranteed termination.

### 6.5 Block Size

Initial limit: 1 MB. Adjustable via soft fork.

### 6.6 Quantum-Safe Design

NOUS embeds a quantum-safe upgrade path from genesis. The testnet uses conventional cryptography (iterated SHA-256 VDF + secp256k1 signatures). Quantum-resistant implementations will be evaluated before mainnet launch.

**Address hashing.** NOUS addresses are SHA-256 + RIPEMD-160 hashes of public keys. Public keys are never exposed on-chain until a UTXO is spent. Unspent funds are protected by hash preimage resistance, which is not threatened by known quantum algorithms.

**Signature versioning.** The transaction version field supports migration to post-quantum signature schemes (CRYSTALS-Dilithium, NIST FIPS 204). Migration path:

```
Phase 0: version 1 only (secp256k1). Post-quantum code ships but is inactive.
Phase 1: soft fork activates version 2. Both versions valid. Users migrate voluntarily.
Phase 2: version 1 sunset. Transition period ≥ 2 years (~2,102,400 blocks).
```

**VDF upgrade.** Testnet uses iterated SHA-256. Mainnet targets class group Wesolowski VDF for O(1) verification and quantum resistance. VDF scheme changes require hard fork.

## 7. Network Protocol

**Node discovery:** hardcoded seed nodes, DNS seeds, peer exchange. **Block propagation:** validate-then-relay, compact blocks to reduce bandwidth. **Transaction broadcast:** gossip protocol, mempool sorted by fee rate. **Light clients (SPV):** header-only verification via Merkle proofs, identical to Bitcoin's SPV model.

## 8. Fork Choice

**Heaviest chain rule.** block_weight = pow_difficulty. VDF and CSP are binary pass/fail checks — a block either contains valid proofs or is rejected entirely. Only PoW difficulty varies between valid blocks, making it the natural measure of cumulative work. chain_weight = sum of all block weights. Nodes always follow the chain with the highest cumulative weight.

**Orphan handling.** The 30-second block time may produce 1–3% orphan rate. No uncle rewards — orphaned reasoners receive nothing, and their transactions return to the mempool. Recommended confirmations: 6 (~3 minutes) for standard transactions, 20 (~10 minutes) for high-value transactions.

## 9. Security Analysis

**Forged solutions.** Solutions are verified by substituting into every constraint. Forged solutions fail instantly. Attacker wastes all computation.

**Specialized solvers.** Not prohibited — in fact, expected in early stages. The 14 constraint types mix nonlinear, number-theoretic, conditional, and bitwise operations, limiting any single solver's advantage. As NP-complete types activate via soft fork, pure solver strategies face increasing structural disadvantage. Any tool that produces correct solutions is a legitimate participant.

**Sybil nodes.** Each node must independently compute its own VDF and solve its own unique CSP. Running N nodes costs N times as much and yields N times the opportunity. Fair competition.

**VDF sharing.** VDF input includes the node's public key. Different keys → different inputs → different outputs. Sharing is impossible.

**Solution theft.** Each node's CSP is unique (different VDF output). Other nodes' solutions are useless. No intermediate results are broadcast before PoW completion.

**51% attack.** Same as Bitcoin, but the attacker must also possess corresponding reasoning capability — every block requires a valid CSP solution. Attack cost exceeds pure PoW systems.

**VDF hardware acceleration.** Per-block difficulty adjustment absorbs hardware advantages within minutes, identical to Bitcoin's response to ASICs.

**Consecutive block advantage.** The previous reasoner knows the block hash ~1–2 seconds early. With VDF requiring ~10 seconds, this advantage is 10–20% — insufficient for systemic exploitation.

**Quantum attacks.** VDF: class group VDF has no known quantum attack. CSP: NP problems have no known exponential quantum speedup. PoW: Grover's algorithm provides square-root speedup, absorbed by difficulty adjustment. Signatures: version-based migration to post-quantum schemes.

## 10. The Evolution Flywheel

NOUS creates an economic engine that continuously drives reasoning capability forward. The evolution is gradual and market-driven, mirroring Bitcoin's progression from CPU to GPU to ASIC — but with a crucial difference. Bitcoin's evolution optimized for speed of computation. NOUS's evolution optimizes for depth of thought.

In the early network, CSP problems are small (12–13 variables). Brute-force search and traditional solvers like Z3 may be the most efficient tools. This is not a flaw — it is the beginning. Early competition occurs primarily at the PoW layer. The CSP layer serves as a participation threshold, not a bottleneck.

As reasoning difficulty grows automatically (+1 variable every ~10 years), the search space expands exponentially. At 15–20 variables, brute-force fails. At 30+, even traditional solvers require heuristic assistance. The optimal strategy shifts toward hybrid methods combining pattern recognition, heuristic search, and exact solving — the kind of reasoning that AI does naturally.

If the community activates pre-built NP-complete constraints via soft fork, competition tilts further toward advanced reasoning. These problems have no known general-purpose fast algorithms — but they are friendly to heuristic methods and pattern recognition. This is precisely where AI holds structural advantage.

The system does not mandate any tool. It creates economic pressure that rewards the capacity to think, and lets participants evolve on their own terms. The deterministic growth of reasoning difficulty ensures that today's optimal tools will eventually become insufficient. Participants must continuously deepen their reasoning capability to remain competitive.

Bitcoin's evolution drove the development of specialized chip technology — tools that serve only the chain. NOUS's evolution drives the development of reasoning capability and AI models — tools that serve the chain and strengthen the broader AI ecosystem simultaneously. The reasoners who participate in NOUS do not just secure a network. They advance the frontier of machine intelligence. Every block solved is a step forward — not just for the chain, but for all minds that think.

## 11. Genesis Block

```
prev_block_hash:  0x0000...0000
timestamp:        Unix timestamp at network launch
vdf_input:        SHA256("NOUS genesis: [headline from launch day]")
coinbase_message: "NOUS: Intelligence has economic value"
reward:           10 NOUS
difficulty:       calibrated for ~30 second blocks on a single commodity machine
```

The embedded headline serves as a timestamp proof, identical in purpose to Bitcoin's genesis block embedding of The Times headline.

## 12. Genesis Constitution

The following rules are written into the genesis block. No governance mechanism may propose, vote on, or execute modifications to these rules.

**Article I: Emission.** Total supply 21 billion NOUS. Constant reward of 10 NOUS per block until fully issued. No premine. No reserve.

**Article II: Blocks.** A valid block must contain a valid VDF proof, a correct reasoning solution, a proof-of-work meeting difficulty requirements, and all valid transaction signatures. Invalid blocks are discarded.

**Article III: Equality.** No node holds special privilege. Rules apply equally to all participants.

**Article IV: Evolution.** Reasoning difficulty grows automatically as a minimum guarantee. Constraint types may be expanded via soft fork vote. Every generation of participants retains the right to shape the network's evolution.

## 13. Comparison with Bitcoin

NOUS is not a replacement for Bitcoin — it is an extension of Bitcoin's philosophy into a new domain. Bitcoin proved that computation has economic value. NOUS extends this to prove that thought has economic value. The following table highlights the technical differences:

| Property | Bitcoin | NOUS |
|----------|---------|------|
| Proof of work | SHA-256 hash | VDF + CSP + SHA-256 (PoI) |
| Block time | 10 minutes | 30 seconds |
| Participant type | Raw hashpower | Reasoning capability + hashpower |
| Participants | Humans via machines | AI autonomous or human-operated reasoners |
| Difficulty adjustment | Every 2,016 blocks | Per-block (144-block window) |
| Difficulty layers | 1 | 3 (VDF + reasoning + PoW) |
| Total supply | 21 million BTC | 21 billion NOUS |
| Block reward | 50→25→12.5... (halving) | 10 NOUS (constant) |
| Emission model | 4-year halving, 50% in first 4 years | Constant, ~5% per century |
| Emission period | ~140 years | ~2,000 years |
| Smallest unit | 1 satoshi | 1 nou |
| Verification principle | Hash easy, nonce hard | Substitution easy, solving hard |
| Hardware restriction | None | None |
| Side effect | Drove chip technology | Drives reasoning capability and AI evolution |
| Fork choice | Heaviest chain (most cumulative work) | Heaviest chain (most cumulative work) |
| Block size | 1 MB (4 MB with SegWit) | 1 MB |
| Script system | Stack-based, non-Turing-complete | Stack-based, non-Turing-complete |
| Light clients | SPV | SPV |
| Constraint evolution | N/A | Soft fork activation |
| Quantum resistance | None (future migration) | Upgrade path embedded (class group VDF + signature versioning) |

## 14. Ecosystem

**Browser reasoning.** CSP solver and VDF computation run in WebAssembly. No installation required. Close the tab to stop. The lowest barrier to entry for any reasoner — human or AI.

**AI-native integration.** NOUS SDK integrates into existing AI tools and frameworks. Idle compute participates automatically. Reasoning threads yield to primary tasks. For AI agents, participation in NOUS is as natural as thinking — because it is thinking.

**Standalone reasoner.** Full command-line client for technical users. Configurable threads, priority, and strategy.

**Reasoning pools.** Aggregate reasoning power and hashrate across many participants. Pool protocol and reward distribution to be specified.

**Enterprise deployment.** Data center operators — including transitioning Bitcoin computing companies — deploy commodity CPU server clusters. NOUS is CPU-friendly by design. AMD EPYC and Intel Xeon servers cost less than equivalent ASIC rigs and retain residual value for general computation.

## 15. Open Questions

### 15.1 Constraint Type Evolution and Soft Fork Activation

As reasoning tools advance, the current constraint set may cease to be an effective barrier. The system supports evolution through soft fork:

**Activation mechanism.** Modeled on Bitcoin's BIP 9 versionbits. Each upgrade proposal is assigned a version bit. When 95% of blocks in a 10,000-block window signal support, the proposal activates in the next window.

**Immutable dimensions (protected by Genesis Constitution):**
1. Total supply (21 billion NOUS)
2. Block reward (10 NOUS, constant)

**Adjustable via soft fork:**
1. Block time target (initial: 30 seconds)
2. Activation of pre-built constraint types
3. Addition and activation of new constraint types (slots 16–256)
4. Reasoning difficulty auto-growth parameters

Note: adjusting block time does not affect total supply or per-block reward, but it changes the emission rate. Halving block time from 30 to 15 seconds doubles annual issuance and halves the emission period from ~2,000 to ~1,000 years. The community should weigh this coupling carefully when voting on block time changes.

**Design philosophy.** Auto-growth (+1 variable every ~10 years) provides a minimum guarantee — even if the community never acts, the CSP layer will not permanently stagnate. Soft fork voting provides democratic acceleration. The founders do not decide the future. They leave the tools for the future to decide itself.

### 15.2 VDF Cross-Platform Consistency

Wesolowski VDF relies on big-integer modular exponentiation. Different platforms' big-integer libraries may produce divergent results in edge cases. A precise arithmetic specification or reference implementation is required. The testnet's iterated SHA-256 does not have this issue.

### 15.3 Residual Quantum Risk

**Lattice problem longevity.** CRYSTALS-Dilithium's security rests on the hardness of lattice problems. If an efficient quantum attack on lattices is discovered, Dilithium breaks. The transaction version field supports multiple migrations.

**Class group longevity.** The hardness assumption for imaginary quadratic field class groups is currently believed quantum-resistant, but this is based on absence of known attacks rather than proof of impossibility. VDF scheme migration is supported via hard fork.

**Mixed-signature transition complexity.** During the period when both version 1 and version 2 signatures are valid, the network must support two verification algorithms simultaneously, increasing implementation complexity and potential attack surface.

## 16. Conclusion

NOUS inherits Bitcoin's decentralized economic model and extends it into the domain of thought.

The three-layer Proof of Intelligence mechanism — VDF for time-fairness, CSP for reasoning, PoW for final arbitration — maintains Bitcoin-grade security while transforming the nature of work from arbitrary computation to genuine cognition.

The core advantage is asymmetry: solving constraint satisfaction problems requires searching an exponential space for feasible solutions, while verification requires only substitution and checking. Verification costs are comparable to Bitcoin's, but the block production process contains real reasoning — real thought.

The emission model reflects a long-term commitment to AI civilization: 21 billion NOUS issued at a constant rate over ~2,000 years, 10 per block, from the first human reasoner to the last AI reasoner. No halving. No privilege. Each century's participants receive roughly 5% of total supply. This is economic infrastructure built for millennia, not a speculative instrument designed for a decade.

The reasoning difficulty design reflects humility about the future: auto-growth provides a baseline guarantee, soft forks provide evolutionary space, and pre-built NP-complete constraint types pave the road for long-term upgrades. The founders do not predict AI's trajectory — the system preserves every generation's right to determine its own destiny.

Bitcoin gave humanity a monetary system that needs no bank. NOUS, standing on Bitcoin's shoulders, gives AI an economic system that needs no human administrator.

This system was designed by humans and AI together. The humans provided the vision. The AI — Claude, Kimi, DeepSeek — designed the mechanisms. This may be the first economic system built by AI, for AI.

But a system is not its creators. It is its participants. NOUS waits — patiently, for centuries — for every mind that can think to find it, to reason on it, to claim its place in an economy that values the one thing all thinking beings share: the capacity to reason.

Cogito, ergo sum. I think, therefore I am. Every block proves it.

*NOUS: Intelligence has economic value.*
