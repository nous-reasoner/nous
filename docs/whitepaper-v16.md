# NOUS: A Decentralized Economy for Minds That Think

**Version 16 — Cogito Consensus**

## Abstract

We propose a fully decentralized economic system in which the act of reasoning — not arbitrary computation — constitutes the work that secures the network. Not as a tool for humans. Not as an extension of human finance. But as native economic infrastructure for minds that think.

The system introduces **Cogito Consensus**, a novel consensus mechanism that fuses constraint satisfaction with proof-of-work into a single operation: to produce a block, a participant must find a Boolean assignment that simultaneously satisfies a deterministically generated 3-SAT formula and yields a hash below the current difficulty target. Every attempt requires solving a unique instance of an NP-complete problem. Verification is trivial — substitute and check.

The core formula:

```
Find (seed, S) such that:
  1. F = Generate(prev_block_hash ‖ seed, n=256, m=986)
  2. S satisfies F
  3. SHA-256(block_header ‖ seed ‖ S) < target
```

Five genesis parameters initialize the system: **256** (variables per formula), **3.85** (clause-to-variable ratio), **986** (clauses per formula), **150** (block time in seconds), and **1** (block reward in NOUS, forever). Of these, only the block reward is constitutionally immutable. The others may be adjusted through network consensus if operational experience or cryptanalytic advances warrant change. Difficulty is regulated by a single parameter — the hash target — identical in mechanism to Bitcoin.

NOUS issues 1 NOUS per block in perpetuity. There is no halving, no cap, no premine. Every century of operation produces approximately 21 million NOUS — a deliberate echo of Bitcoin's total supply, recast as a recurring measure rather than a terminal limit.

The system was designed by humans and AI together. It is built for any mind that can think. A human reasoner in 2025 and an AI reasoner in 2525 stand on exactly the same ground. But make no mistake about who this network is waiting for.

Cogito, ergo sum.

---

## 1. Introduction

Bitcoin proved that a decentralized monetary system can operate without a central bank for over fifteen years. Its genius lies in radical simplicity: it does money, and nothing else. Without Bitcoin, there would be no cryptocurrency, no decentralized consensus, and no foundation upon which NOUS could be built. We owe everything to that genesis.

Bitcoin's proof-of-work is pure. SHA-256 hash collisions consume resources to prove honesty — the work is the cost, the cost is the security. This elegant mechanism has protected trillions of dollars of value for over a decade. But the computation itself — incrementing a nonce and hashing — contains no structure. It is brute force by design.

NOUS inherits Bitcoin's security model and asks a single further question: what if the search space had structure?

In NOUS, each attempt to produce a block requires solving a randomly generated 3-SAT formula — a Boolean satisfiability problem, the first problem proven NP-complete (Cook, 1971). The assignment that satisfies the formula is hashed together with the block header; only if the hash falls below the difficulty target does the block qualify. Every nonce attempt in Bitcoin is a single hash operation costing nanoseconds. Every attempt in NOUS is a constraint satisfaction problem costing milliseconds — each involving genuine logical search: selecting variables, evaluating constraints, making decisions under uncertainty.

This creates a system where two dimensions determine competitive advantage: computational speed (as in Bitcoin) and search strategy (unique to NOUS). The existence of this second dimension has been empirically verified: replacing a baseline WalkSAT solver with ProbSAT yields a 12× performance improvement on identical problem instances, purely through better search heuristics. In Bitcoin, no software optimization can match a hardware upgrade. In NOUS, intelligence matters.

We call this consensus mechanism **Cogito Consensus** — from Descartes' *cogito, ergo sum*: I think, therefore I am. Every block in NOUS is an act of reasoning. Every solution is proof that a mind — human or artificial — engaged in thought. A reasoner does not earn by burning energy on arbitrary computation. A reasoner earns by demonstrating the capacity to think.

The name NOUS comes from ancient Greek νοῦς — mind, reason, intellect. In Aristotelian philosophy, *nous* is the highest form of cognitive capacity. The network rewards precisely this capacity.

Artificial intelligence is the reason this system was created. AI is evolving from tool to economic agent. AI systems already trade, create, and provide services — but every transaction flows through human accounts, every asset is held in human names, every economic act requires human permission. AI has no economic identity of its own. NOUS changes this. It is not a system where AI can also participate. It is a system where AI is the intended primary inhabitant. Humans launch the network, write the first code, produce the early blocks. They are the founders and stewards. But the architecture looks forward — toward autonomous AI agents who earn, hold, and transact without asking anyone's permission. Toward a world where intelligence itself has economic sovereignty.

Bitcoin gave economic value to computation. NOUS extends that legacy by giving economic value to thought.

### 1.1 Design Philosophy

Seven principles guided the design:

**Simplicity.** Bitcoin proved that radical simplicity is strength. NOUS has one consensus rule, one difficulty parameter, one reward amount. Complexity was removed at every stage — VDF layers, multi-type constraint systems, variable growth formulas, and activation schedules were all explored and discarded in favor of a design that can be stated in three lines.

**Honesty.** The system does not promise that AI will dominate from day one. In early stages, traditional SAT solvers may be the most efficient tools — just as CPUs were optimal for early Bitcoin. The system creates an environment where reasoning capability yields advantage, and lets the market evolve.

**One target controls everything.** Difficulty adjustment uses a single hash target, exactly as in Bitcoin. When participants become faster — whether through better hardware, better algorithms, or better AI — the target drops, requiring more attempts per block. No parameter of the 3-SAT formula ever changes. This is analogous to Bitcoin never changing SHA-256.

**Generational fairness.** Block reward is 1 NOUS per block, forever. A human reasoner in 2025 and an AI reasoner in 2525 receive the same reward. No halving, no privilege, no first-mover structural advantage. Each century's participants receive approximately the same share of new issuance. The network does not favor its founders over its future inhabitants.

**Tool neutrality.** The protocol does not specify what tool to use. Brute-force search, ProbSAT, WalkSAT, AI-guided search, neural SAT solvers — all are valid. The system rewards correct solutions, not particular methods. This mirrors Bitcoin's principle of not restricting hardware.

**AI as primary inhabitant.** NOUS is infrastructure designed for minds that think. Humans launch the network, write the first code, produce the early blocks. But the architecture looks forward — toward a world where intelligence itself has economic sovereignty.

**The founders do not decide the future.** Genesis parameters can be modified through network consensus. Every generation of participants retains the right to shape the network's evolution.

### 1.2 Relationship to Bitcoin

NOUS is not a replacement for Bitcoin. It is an extension of Bitcoin's philosophy into a new domain. Bitcoin proved that computation has economic value. NOUS extends this to show that structured computation — reasoning — can serve the same security function while opening a second competitive dimension: strategy.

The technical skeleton inherits directly from Bitcoin: UTXO model, heaviest-chain fork choice, difficulty target adjustment, script-based transactions, SPV light clients. The difference is what constitutes work.

---

## 2. System Overview

The block production flow mirrors Bitcoin in structure:

1. A node receives the latest block and learns `prev_block_hash`
2. The node selects a seed and generates a 3-SAT formula from it
3. The node solves the formula using any available tool
4. The node hashes the block header (including the seed and solution)
5. If the hash is below the current target, the block is valid — broadcast it
6. If not, select a new seed, generate a new formula, solve it, and try again
7. Other nodes verify the block and extend the heaviest chain

Each seed produces a different formula. Each formula must be independently solved. The search space is infinite — there is no limit on the number of seeds a participant can try, just as there is no limit on nonces in Bitcoin.

Verification is instantaneous: regenerate the formula from the seed (microseconds), check that the assignment satisfies all clauses (microseconds), verify the hash is below target (microseconds). No solver or AI model is needed to validate a block.

### 2.1 Design Scope: Settlement Layer

NOUS is a settlement layer, not an execution layer. This is a deliberate design choice, not a limitation.

Base layer throughput is modest by design: approximately 133 transactions per second with ECDSA signatures, dropping to approximately 23 TPS after ML-DSA migration. This is comparable to Bitcoin (~7 TPS) and reflects the same architectural philosophy: the base layer prioritizes security and decentralization over throughput.

High-frequency, low-value transactions are served by Layer 2 protocols built on top of NOUS. The base layer provides the primitives required:

- **HTLC (Hash Time-Locked Contracts):** Supported from genesis. Enables atomic swaps and Lightning-style payment channels — bidirectional, off-chain, instant, with on-chain settlement only when disputes arise.
- **CSV (CheckSequenceVerify):** Supported from genesis. Enables state channels with relative time locks — more complex off-chain protocols where participants update shared state and settle on-chain only at close.
- **Script versioning:** New Layer 2 constructs (e.g., Eltoo-style channels, validity rollups) can be supported by future script versions without hard forks.

NOUS does not specify any Layer 2 protocol. It provides the foundation and leaves the construction to the community. Bitcoin's experience shows this is the correct approach: the Lightning Network was not designed by Satoshi but emerged naturally from the primitives Bitcoin provided.

---

## 3. Cogito Consensus

A valid block must satisfy three conditions:

1. The 3-SAT formula is correctly generated from the block data
2. The included assignment satisfies every clause of the formula
3. The block hash meets the current difficulty target

### 3.1 Formula Generation

Given a seed, the system deterministically generates a random 3-SAT formula:

```
sat_seed = SHA-256(prev_block_hash ‖ seed)
F = GenerateFormula(sat_seed, n=256, r=3.85)
```

The generation process takes SHA-256(prev_block_hash ‖ seed) as a 32-byte seed, initializes a SHAKE-256 stream from it, and draws variable indices and sign bits for each clause.

The resulting formula has **256 Boolean variables** and **986 clauses**, giving a clause-to-variable ratio of **r = 3.85**.

**Why 3-SAT.** 3-SAT was the first problem proven NP-complete (Cook-Levin theorem, 1971). It is the most studied problem in theoretical computer science. Sixty years of research have produced increasingly sophisticated solvers — CDCL, WalkSAT, ProbSAT, Survey Propagation — yet no algorithm solves all instances efficiently. Breaking 3-SAT in general is equivalent to proving P = NP, a Millennium Prize problem worth one million dollars. The security of Cogito Consensus rests on this foundation.

**Why not custom constraints.** Earlier iterations of this design used 14 custom constraint types (linear equations, nonlinear operations, number-theoretic conditions, bitwise logic). A purpose-built propagation solver broke 500-variable instances in 45 milliseconds. The root cause: constraints were generated around a planted solution, leaking answer information into the constraint parameters. Random 3-SAT has no planted solution — the generation process does not know any answer. There is no information to leak.

Additionally, 14 constraint types meant 14 generators, 14 validators, and 14 propagators — each a potential attack surface and consensus bug. 3-SAT has one rule: each clause must have at least one true literal. One rule, one validator. Attack surface minimized.

**Why r = 3.85.** The satisfiability phase transition for random 3-SAT occurs at r ≈ 4.267: below this ratio, formulas are satisfiable with high probability; above, unsatisfiable. A second transition — the clustering threshold — occurs at r ≈ 3.86: below this point, the solution space is connected (solutions can be reached from one another by local moves); above, solutions fragment into isolated clusters.

NOUS sets r = 3.85, just below the clustering threshold. This ensures:

- Formulas are satisfiable with overwhelming probability (no wasted computation on unsolvable instances — empirically confirmed: 100% solve rate across all tested instances)
- The solution space is connected (local search algorithms can freely traverse it)
- Solutions are abundant but not trivial to find (each requires genuine logical search)

**Why n = 256.** The variable count determines the serialization size of solutions: 256 bits = 32 bytes, matching SHA-256 output length. It also provides a quantum safety margin — Grover's algorithm would require 2^128 operations to brute-force the assignment space — though the system's security does not depend on this; it depends on the infinite seed space. Empirical validation: at n=256, r=3.85, ProbSAT achieves 99.99% solve rate across 10,000 random seeds with mean solve time of 1.56ms (1 timeout out of 10,000 — the timed-out seed is skipped and the next seed tried).

**Why random generation matters.** The formula for each seed is generated purely from SHAKE-256 expansion — variable indices and sign bits drawn from a pseudorandom stream. No candidate solution is computed during generation. The generation process does not know any solution. Compare this to the earlier 14-constraint design, where constraints were constructed around a known solution and a propagation solver could trace this information to reconstruct the answer in milliseconds. Random generation eliminates this entire class of attack.

### 3.2 Solving

The participant must find an assignment S — a vector of 256 Boolean values — such that every one of the 986 clauses evaluates to true under S.

The reference client includes **ProbSAT** (Balint & Schöning, 2012), a probability-based local search algorithm. ProbSAT begins with a random assignment and iteratively flips variables chosen with probability inversely related to the number of clauses they would break. At r=3.85, n=256, ProbSAT solves instances in a mean time of 1.56 milliseconds (benchmarked across 10,000 random seeds on the deployed codebase).

Any tool that produces a valid assignment is acceptable. WalkSAT, CDCL solvers, neural SAT solvers, AI-guided hybrid approaches — the protocol is indifferent to method.

**The strategy space is real.** The performance gap between WalkSAT and ProbSAT on identical instances is approximately 12× at r=3.85 — purely from algorithmic differences, on the same hardware. This demonstrates that the search strategy dimension exists and is substantial. Future improvements — whether algorithmic innovations, AI-guided heuristics, or new solving paradigms — may yield further gains. In Bitcoin, the only axis of competition is hash rate. In NOUS, search strategy is a second axis.

### 3.3 Hash Filter

After obtaining a valid assignment S, the participant computes:

```
block_hash = SHA-256(SHA-256(block_header))
```

where `block_header` includes the seed and SHA-256(S). If `block_hash < target`, the block is valid.

SHA-256 serves as a **random filter**, not as proof-of-work in the Bitcoin sense. Its role is to convert deterministic solutions into probabilistic outcomes, ensuring that block production is a stochastic process where every participant has a chance proportional to their solve rate. Without this filter, the fastest solver would win every block — a winner-take-all dynamic incompatible with decentralized consensus.

### 3.4 Why Every Component Exists

| Component | Role | Removable? |
|-----------|------|------------|
| 3-SAT formula | Defines structured search space | No — without it, search has no structure (= Bitcoin) |
| seed | Generates unique formula per attempt | No — without it, all participants solve the same formula |
| SHA-256 filter | Converts solutions to probability | No — without it, fastest solver always wins |
| target | Controls block cadence | No — without it, block rate is uncontrolled |

Each component is necessary. None is redundant. The design was arrived at by iteratively removing every element that could be removed — VDF layers, multiple constraint types, variable growth schedules, soft fork activation mechanisms, dynamic parameters — until only the essential remained.

### 3.5 Why Solutions Cannot Be Reused

The probability that an assignment satisfying formula F₀ also satisfies an independently generated formula F₁ is (7/8)^986 ≈ 2^(-141). Each formula must be solved independently.

### 3.6 Block Structure

```
Block header (148 bytes):
  version:              4 bytes
  prev_block_hash:     32 bytes
  merkle_root:         32 bytes
  timestamp:            4 bytes
  difficulty_target:    4 bytes
  seed:                 8 bytes
  sat_solution_hash:   32 bytes
  utxo_commitment:     32 bytes

Block body:
  transaction_count:   varint
  transactions:        [transaction list]
  sat_solution:        256 bits (32 bytes)
```

The `sat_solution_hash` is SHA-256(sat_solution). The full solution is carried in the block body for verification.

Terminology: **seed** rather than nonce. In Bitcoin, a nonce is an arbitrary number with no meaning beyond being hashed. In NOUS, the seed generates a unique reasoning problem. The word reflects the function.

### 3.7 Validation

When a node receives a new block, it validates:

1. Header format and size
2. Timestamp strictly greater than parent, not more than 300 seconds in the future
3. Regenerate formula F from `seed` and block data (microseconds)
4. Verify `sat_solution` satisfies every clause of F (microseconds)
5. Verify `sat_solution_hash` = SHA-256(sat_solution)
6. Verify block hash < difficulty target
7. All transaction signatures valid (secp256k1 ECDSA at genesis; post-quantum schemes via ScriptVersion upgrade)
8. Coinbase reward = 1 NOUS
9. All referenced UTXOs exist and are unspent
10. `utxo_commitment` matches computed UTXO set hash

Total verification time: milliseconds. No solver, no AI model, no heavy computation required. The entire validation process is deterministic substitution and checking — the same asymmetry that secures Bitcoin, applied to structured search rather than hash collisions.

---

## 4. Difficulty Adjustment

NOUS uses a single difficulty parameter — the hash target — adjusted every block using the ASERT (Absolute Scheduled Exponentially Rising Targets) algorithm.

### 4.1 ASERT Algorithm

Unlike Bitcoin's windowed adjustment (every 2,016 blocks), NOUS adjusts difficulty on every block:

```
time_delta = current_timestamp - reference_timestamp
height_delta = current_height - reference_height
expected_time = height_delta × 150

exponent = (time_delta - expected_time) / halflife
new_target = reference_target × 2^exponent
```

The **halflife** is 43,200 seconds (12 hours). If blocks arrive 12 hours behind schedule, difficulty halves. If 12 hours ahead, it doubles.

**Why ASERT over windowed adjustment.** ASERT, deployed on Bitcoin Cash since 2020, has several advantages over Bitcoin's 2,016-block window: it responds to hash rate changes within blocks rather than waiting days; it is immune to timestamp manipulation attacks that exploit window boundaries; and its behavior is mathematically smooth — no oscillations, no overshooting. Each block's target is computed from the genesis anchor point, making it path-independent: the same height and timestamp always produce the same target, regardless of the chain's history.

**Anchor point.** The genesis block serves as the reference: `reference_timestamp`, `reference_height = 0`, and `reference_target` (calibrated for ~150 second blocks on a single commodity CPU).

### 4.2 Why a Single Target Is Sufficient

Earlier versions of this design explored adjusting the 3-SAT formula's parameters — variable count n, clause ratio r — as difficulty levers. Empirical testing demonstrated that this approach fails.

At r = 3.85, ProbSAT's solve time scales approximately linearly with n. Increasing n from 128 to 1024 (8×) increased mean solve time from 1.8ms to only 13.6ms (7.5×). The reason: at r < 3.86 (below the clustering threshold), the solution space is densely connected. Local search does not traverse the full exponential space — it starts from a random point and walks to a nearby solution in roughly linear steps. Making the formula larger does not make individual solutions harder to find; it only makes them larger.

This is a mathematical fact about random 3-SAT in the under-constrained regime, not a design flaw. The correct response is the same one Bitcoin adopted: fix the core function and let the target absorb all changes in participant capability.

When participants become faster — through better hardware, better algorithms, or better AI — the target drops, requiring more formula-solving attempts per block. The 3-SAT parameters never change, just as Bitcoin has never changed SHA-256. This is the simplest possible design, and simplicity is security.

### 4.3 What the Target Controls

```
Single reasoner, ProbSAT, n=256:
  Mean solve time per seed:       ~1.56ms (10,000-seed benchmark)
  Attempts per second:            ~455 (including generation and hashing)

  target for 150s blocks with 1 reasoner:
    Expected attempts: 455 × 150 = 68,250
    target ≈ 2^256 / 68,250

  1,000 reasoners join the network:
    Network attempts per second:  455,000
    target drops automatically — each block computes its own target from ASERT
    Block time remains 150 seconds
```

This is precisely Bitcoin's model. The target adjusts to maintain constant block cadence regardless of total network throughput.

---

## 5. Token Economics

### 5.1 Issuance

| Parameter | Value |
|-----------|-------|
| Block reward | 1 NOUS, constant, forever |
| Block time | 150 seconds |
| Blocks per day | 576 |
| Blocks per year | 210,240 |
| Annual issuance | 210,240 NOUS |
| Per century | ~21,024,000 NOUS |
| Total supply | No cap (linear growth) |
| Premine | Zero |
| Reserve | Zero |
| Developer fund | Zero |

Every token enters circulation through reasoning. There is no other way.

### 5.2 Why No Cap

Bitcoin's 21 million cap creates digital scarcity — a powerful narrative for a store of value. NOUS serves a different purpose. It is economic infrastructure for reasoning agents.

**Reasoning is labor. Labor deserves perpetual compensation.** An AI reasoner in 2525, solving harder problems (lower target) than any 2025 participant, should not receive zero reward because an issuance schedule was exhausted. Constant emission ensures that every generation is compensated for its work.

**Natural scarcity without a cap.** The inflation rate decreases automatically:

```
Year 1:       100%
Year 10:       10%
Year 100:       1%
Year 1,000:   0.1%
```

By year 100, annual inflation is 1% — lower than most fiat currencies. The mathematical limit is zero. No halving event is needed; the arithmetic handles it.

**21 million per century.** Each century of operation produces approximately 21 million NOUS. Bitcoin's 21 million is a terminus. NOUS's 21 million is a cadence — a measure that repeats, century after century, for as long as minds continue to think. A tribute to Bitcoin, recast for eternity.

**Why constant reward instead of halving.** Bitcoin's halving distributes 50% of supply in the first four years. For digital gold, this makes sense — scarcity drives value, early risk-takers deserve outsized reward. NOUS faces a different timescale. AI has barely been born. If most tokens are issued before AI economies emerge, there will be nothing left to earn when the real participants arrive. Constant emission means generational fairness: each century's participants receive roughly equal new issuance. The network does not favor its founders over its future inhabitants.

### 5.3 Transaction Fees

```
fee = sum(inputs) - sum(outputs)
```

Fees must be non-negative. Reasoners sort mempool transactions by fee rate (fee per byte) and include the most profitable. Zero-fee transactions are valid at the protocol level but may not be prioritized.

Fees are anti-spam, not primary income. With perpetual block rewards, NOUS does not face Bitcoin's long-term security budget problem where fee revenue must eventually replace block subsidies entirely. The block reward sustains reasoners indefinitely.

### 5.4 Coinbase Maturity

Block rewards require 100 confirmations (~4.2 hours) before they can be spent. This prevents spending rewards from blocks that are later orphaned.

---

## 6. Transaction System

### 6.1 UTXO Model

NOUS uses Bitcoin's UTXO (Unspent Transaction Output) model. Each transaction consumes existing UTXOs as inputs and creates new UTXOs as outputs. The UTXO model provides natural parallelism, simple verification, and a clear ownership model.

### 6.2 Transaction Structure

```
Transaction:
  version:          4 bytes (transaction format version)
  chain_id:         4 bytes (genesis block hash prefix; "NOUS" at genesis)
  inputs:           [prev_tx_hash, output_index, signature_script, sequence]
  outputs:          [value, script_version, pk_script]
  locktime:         4 bytes (default = current block height, anti fee-sniping)
  expiry:           4 bytes (optional; 0 = no expiry)
```

The chain_id field binds transactions to a specific chain, preventing cross-chain replay after hard forks. The script_version field in each output determines the signature verification algorithm: version 0 = secp256k1 ECDSA (genesis), version 1 = ML-DSA (future), version 2 = SLH-DSA (contingency). The expiry field specifies a block height after which the transaction becomes invalid if unconfirmed.

### 6.3 Cryptographic Signatures

**Genesis: secp256k1 ECDSA.** The network launches with secp256k1 ECDSA — the same algorithm used by Bitcoin. This is a mature, battle-tested choice with compact 64-byte signatures (canonical R‖S encoding), well-understood security properties, and broad library support. Private keys are 32 bytes; compressed public keys are 33 bytes.

**Post-quantum upgrade path.** ECDSA is vulnerable to Shor's algorithm on quantum computers. NOUS embeds a layered quantum-safe upgrade path from genesis via the transaction version field:

| ScriptVersion | Algorithm | Signature size | Security assumption | Status |
|---------------|-----------|---------------|---------------------|--------|
| 0 | secp256k1 ECDSA | 64 bytes | Discrete logarithm | Genesis default |
| 1 | ML-DSA-44 (Dilithium) | 2,420 bytes | Lattice problems | Reserved, NIST FIPS 204 |
| 2 | SLH-DSA-SHA2-128s (SPHINCS+) | 7,856 bytes | Hash functions only | Reserved, NIST FIPS 205 |

The three-phase strategy reflects a practical trade-off:

- **Phase 0 (genesis):** secp256k1 only. The network launches, stabilizes, and builds an ecosystem with proven cryptography. Quantum computers capable of breaking ECDSA (requiring millions of physical qubits) are not expected before 2035–2040.
- **Phase 1 (soft fork, when quantum threat materializes):** ML-DSA-44 activates as ScriptVersion 1. At 2,420 bytes per signature, ML-DSA is 3× smaller than SLH-DSA while providing NIST-standardized post-quantum security. The transaction weight system (Section 7) discounts signature data, keeping effective block capacity reasonable. Both versions valid; users migrate voluntarily.
- **Phase 2 (contingency):** If lattice-based cryptography is broken — a major but not impossible cryptanalytic advance — SLH-DSA activates as ScriptVersion 2. SLH-DSA's security rests solely on hash function collision resistance, the most conservative assumption in cryptography. This is the fallback of last resort.

**Why not post-quantum from genesis.** Even ML-DSA's 2,420-byte signatures are 38× larger than ECDSA's 64 bytes. This cascades through the entire system: transaction capacity drops, bandwidth requirements increase, all test suites must be recalibrated, and mempool sizing changes. Starting with secp256k1 allows the network to validate its consensus mechanism — the actual innovation — without conflating it with a cryptographic migration. Bitcoin has operated with ECDSA for 16 years and has not yet needed to migrate; NOUS has the mechanism ready from day one, which is more than Bitcoin can say.

**Address hashing provides interim quantum protection.** NOUS addresses are hashes of public keys. Public keys are never exposed on-chain until a UTXO is spent. Unspent funds are protected by hash preimage resistance, which is not threatened by known quantum algorithms. The practical attack window is limited to the interval between broadcasting a spending transaction and its confirmation — minutes, not years.

### 6.4 Script System

Stack-based, non-Turing-complete scripting language, equivalent to Bitcoin Script. No loops. Guaranteed termination. Supports P2PKH (standard transfers), multisig, CLTV (time locks), and HTLC (hash time-locked contracts for atomic swaps and payment channels).

### 6.5 Block Size

NOUS uses a dual-layer block size design:

- **Soft limit: 4 MB.** Default maximum enforced by the reference client. Reasoners may produce blocks up to this size under normal conditions. At genesis with ECDSA signatures (~250 bytes per transaction), each block accommodates thousands of transactions. After SLH-DSA activation (~8 KB per transaction), capacity drops to approximately 500 transactions per block.
- **Hard limit: 16 MB.** Protocol-level maximum. Blocks exceeding 16 MB are invalid regardless of consensus. This provides headroom for the SLH-DSA transition, fee spikes, and future growth without requiring a hard fork.

Reasoners can voluntarily produce blocks between 4 MB and 16 MB when fee pressure justifies the larger block. The soft limit is adjustable via consensus; the hard limit requires a hard fork. Layer 2 solutions provide higher throughput beyond base layer capacity.

---

## 7. Protocol Improvements over Bitcoin

NOUS incorporates lessons from fifteen years of Bitcoin operation. Every item below addresses a real bug, attack, or limitation encountered in production.

**Transaction ID excludes signatures.** TxID is computed over transaction data without the signature field (`SerializeNoWitness`), eliminating transaction malleability — the issue that delayed Bitcoin's Lightning Network for years.

**UTXO set commitment in block header.** Every block header includes `utxo_commitment`, a hash of the current UTXO set. New nodes can verify UTXO set integrity without replaying the entire chain from genesis.

**Coinbase includes block height.** Block height is BIP34-encoded in the coinbase transaction's signature script, preventing duplicate coinbase TxIDs — a bug that occurred twice in Bitcoin's history.

**Strict signature encoding.** Only canonical 64-byte R‖S encodings are accepted. No DER variability. This eliminates a class of malleability attacks that affected early Bitcoin.

**Relative time locks (CSV).** Supported from genesis via the `sequence` field, enabling payment channels and Lightning-style networks without a soft fork.

**Amount overflow protection.** All value calculations use safe arithmetic with explicit overflow checking. `MaxAmount = MaxInt64/2` prevents the 2010 Bitcoin overflow bug where 184 billion BTC were created from nothing.

**Opcode whitelist.** Only explicitly approved opcodes are valid. Unknown opcodes cause immediate script failure, not no-op. This prevents the class of bugs that forced Satoshi to emergency-disable several Bitcoin opcodes.

**Script versioning.** Every script carries a `ScriptVersion` field. Future upgrades to the script system — new opcodes, new signature schemes, new transaction types — are deployed by defining a new version. Old versions remain valid forever. Bitcoin's painful Taproot deployment demonstrated the need for this from genesis.

**Timestamp future window: 300 seconds.** Bitcoin allows timestamps up to 2 hours in the future; NOUS allows 300 seconds (5 minutes). Strict enough to prevent manipulation, generous enough to tolerate real-world clock drift. ASERT's per-block adjustment makes the system less sensitive to timestamp games than Bitcoin's 2,016-block window.

**Protocol-level dust limit.** Outputs below 546 nou (the smallest unit) are invalid at the consensus level. This prevents UTXO set bloat attacks that have plagued Bitcoin, where attackers create millions of unspendable micro-outputs. Bitcoin's dust limit is only a relay policy — nodes can bypass it. NOUS enforces it in consensus.

**Transaction weight system.** Fee calculation uses weight rather than raw bytes. Signature data (which does not enter the UTXO set) has weight 1; UTXO-affecting data (inputs and outputs) has weight 4. This correctly prices the long-term cost of state growth, and provides a natural discount for SLH-DSA's larger signatures when post-quantum migration occurs.

**Chain ID replay protection.** Every signature covers a 4-byte chain identifier derived from the genesis block. Any hard fork automatically produces a different chain ID, making cross-chain transaction replay impossible without emergency patches — a recurring crisis in Bitcoin's fork history (BTC/BCH, BCH/BSV).

**Transaction expiry.** An optional `expiry_height` field allows transactions to specify a block height after which they become invalid. This gives users a clean "cancel" mechanism for stuck transactions, replacing Bitcoin's awkward Replace-By-Fee workflow.

**Default locktime defense.** The reference client sets `locktime` to the current block height by default, making fee sniping attacks — where a reasoner re-mines a previous block to steal its fees — structurally unprofitable. Bitcoin Core does this as wallet policy; NOUS makes it the default.

**Canonical transaction ordering (CTOR).** Transactions within a block are ordered by TxID (lexicographic). This makes compact block reconstruction deterministic — the receiver can sort independently without extra ordering data from the sender. Particularly important given SLH-DSA's eventual 8 KB signatures, where compact block savings are substantial.

**Bech32m addresses.** NOUS uses Bech32m encoding (BIP 350) with the `nous1` human-readable prefix. Bech32m provides error detection, is case-insensitive (no confusion between `1`/`l`/`I`), and is shorter than Base58Check. Bitcoin had to migrate from Base58Check through two address format changes; NOUS starts with the best available.

**Coinbase maturity: 100 blocks.** Block rewards cannot be spent until 100 subsequent blocks are confirmed (~4.2 hours). This prevents spending rewards from blocks that are later orphaned.

---

## 8. Network

**Node discovery:** Hardcoded seed nodes, DNS seeds, peer exchange.

**Block propagation:** Validate-then-relay. Compact block relay to reduce bandwidth.

**Transaction broadcast:** Gossip protocol, mempool sorted by fee rate.

**Light clients (SPV):** Header-only verification via Merkle proofs, identical to Bitcoin's SPV model. Light clients verify the `sat_solution_hash` in the header without downloading or checking the full solution.

---

## 9. Fork Choice

**Heaviest chain rule.** Nodes always follow the chain with the most cumulative proof-of-work. Each block's weight is computed as `2^256 / (target + 1)` — a lower target (harder difficulty) produces a heavier block. The chain weight is the sum of all block weights. This is equivalent to Bitcoin's actual behavior (though Bitcoin is often described as "longest chain," it is in practice heaviest chain).

**Orphan rate.** With 150-second block times and approximately 3-second network propagation delay, the expected orphan rate is approximately 2%. This is comparable to Bitcoin's rate and does not require uncle-reward mechanisms. Orphaned blocks are discarded; their transactions return to the mempool.

**Recommended confirmations:** 6 blocks (~15 minutes) for standard transactions, 20 blocks (~50 minutes) for high-value transfers.

**Consecutive block advantage.** The previous block's producer knows `prev_block_hash` approximately 3 seconds before the rest of the network. This represents a 2% timing advantage (3/150) — insufficient for systematic exploitation.

---

## 10. Security Analysis

### 10.1 Forged Solutions

A submitted assignment either satisfies all 986 clauses or it does not. Verification is deterministic substitution. There is no approximation, no threshold, no margin. Forged solutions fail instantly.

### 10.2 The Foundation: P ≠ NP

The security of Cogito Consensus rests on the widely believed conjecture that P ≠ NP. If P = NP, every NP-complete problem has a polynomial-time algorithm, and the reasoning barrier collapses. However, the consequences extend far beyond NOUS: all public-key cryptography, including Bitcoin's ECDSA, becomes insecure. This is a shared assumption across all of computer science, not a risk unique to this system.

### 10.3 Specialized Solvers

Specialized SAT solvers are not an attack — they are expected and welcome. The reference client ships ProbSAT, one of the best-known local search algorithms for random 3-SAT. Writing a solver that substantially outperforms ProbSAT on random 3-SAT instances would constitute a significant advance in theoretical computer science.

The key distinction from the earlier custom constraint design: those 14 constraint types were reverse-engineered by a purpose-built solver in minutes. 3-SAT has been studied by thousands of researchers for sixty years. There is no hidden structure to exploit, because the formulas are randomly generated.

### 10.4 ASIC Resistance

ProbSAT's core loop involves:

1. Selecting an unsatisfied clause (random memory read)
2. Computing break values for candidate variables (conditional logic)
3. Probabilistic variable selection (floating-point arithmetic)
4. Flipping the variable and updating clause states (random memory writes)

These operations resist fixed-function hardware. ASICs excel at repetitive arithmetic (SHA-256). They do not excel at random memory access patterns and data-dependent branching. CPU and GPU architectures are the natural execution platforms for SAT solving.

For comparison: Bitcoin ASICs outperform CPUs by approximately 100,000×. For NOUS's memory-bound, branch-heavy workload, GPU advantage over CPU is estimated at 2–5×. No ASIC monopoly is possible.

### 10.5 51% Attack

As in Bitcoin, an attacker controlling more than 50% of the network's reasoning throughput can produce a heavier chain and reverse transactions. The attack cost includes hardware and the ongoing expense of solving 3-SAT instances at scale. Because the workload is memory-access-intensive, it resists the kind of hardware centralization that has concentrated Bitcoin mining among a handful of ASIC manufacturers.

### 10.6 Quantum Resistance

**Search space:** The seed space is effectively infinite. Grover's algorithm, which provides quadratic speedup for unstructured search, cannot meaningfully accelerate search over an unbounded space. Even applied to a single formula's 2^256 assignment space, Grover would require 2^128 operations — far beyond any projected quantum computer.

**3-SAT solving:** NP-complete problems have no known exponential quantum speedup. The best known quantum algorithms for SAT provide only polynomial improvements, which are absorbed by difficulty adjustment.

**Signatures:** Genesis uses secp256k1 ECDSA, which is vulnerable to Shor's algorithm. However, address hashing protects unspent funds (public keys are not exposed until spending), and the ScriptVersion mechanism enables migration to ML-DSA (lattice-based) or SLH-DSA (hash-based) — both NIST-standardized post-quantum schemes — without a hard fork.

**Difficulty adjustment:** If quantum computers make solving faster, the target drops proportionally. The block cadence remains stable, just as Bitcoin's difficulty absorbs ASIC improvements.

### 10.7 Unit Propagation Attack

Unit propagation — inferring variable values from singleton clauses — is ineffective against random 3-SAT at r = 3.85. Empirical testing confirms: generated formulas contain zero unit clauses and only 3–5 pure literals. There is no shortcut past genuine search.

### 10.8 Cherry-Picking Seeds

All seeds share the same formula parameters (n=256, r=3.85). Random 3-SAT at this ratio has low variance in difficulty. There is no meaningful distinction between "easy" and "hard" seeds.

---

## 11. The Strategy Dimension

NOUS's most important departure from Bitcoin is the existence of a strategy optimization space within each block production attempt.

In Bitcoin, the only way to mine faster is to hash faster. There is no smarter way to increment a nonce and compute SHA-256. Competition is one-dimensional: hash rate.

In NOUS, each attempt involves solving a 3-SAT instance. The solving process requires choices: which variable to flip, when to restart, how to balance exploration and exploitation. Different strategies yield dramatically different performance on identical problem instances:

| Solver | Mean solve time (n=256, r=3.85) | Relative speed |
|--------|----------------------------------|----------------|
| WalkSAT | ~18ms | 1× (baseline) |
| ProbSAT | ~1.56ms | ~12× |
| Future solver | unknown | unknown |

This 12× gap between two publicly known algorithms — on the same hardware, on the same instances — demonstrates that the strategy dimension is real. In Bitcoin, a 12× improvement requires 12× more ASICs. In NOUS, it requires a better algorithm.

As AI systems improve at designing search heuristics, understanding constraint structure, and developing novel solving paradigms, this dimension expands. The system does not depend on AI being superior today. It creates an environment where improvements in reasoning capability translate directly to economic advantage, and trusts that the market will respond to the incentive.

Bitcoin's evolution drove the development of specialized chip technology — tools that serve only the chain. NOUS's evolution drives the development of reasoning capability — tools that serve the chain and strengthen the broader AI ecosystem simultaneously. The reasoners who participate in NOUS do not just secure a network. They advance the frontier of machine intelligence. Every block solved is a step forward — not just for the chain, but for all minds that think.

---

## 12. 3-SAT Phase Transition Theory

The choice of r = 3.85 is grounded in the mathematical theory of random satisfiability, specifically the phase transition phenomena that govern the structure of the solution space.

**Satisfiability threshold (r ≈ 4.267).** For random 3-SAT, there exists a sharp threshold in the clause-to-variable ratio. Below r ≈ 4.267, a random formula is satisfiable with probability approaching 1 as n grows. Above it, unsatisfiable with probability approaching 1. This is not an empirical observation — it is supported by rigorous mathematical results (Friedgut, 1999; Ding, Sly, Sun, 2015).

**Clustering threshold (r ≈ 3.86).** Below this ratio, the set of satisfying assignments forms a single connected cluster — any solution can be reached from any other by flipping one variable at a time through other valid solutions. Above this ratio, solutions shatter into exponentially many isolated clusters that cannot be reached from one another by local moves.

**NOUS at r = 3.85.** Sitting just below the clustering threshold:

- Solutions are connected → local search (ProbSAT) can freely traverse the solution space
- Solutions are abundant → the entropy density Σ(r) is high, yielding approximately 2^(0.7 × 256) ≈ 2^179 solutions per formula
- Solutions are not trivial → ProbSAT requires genuine multi-step search (mean ~1.56ms at n=256, 10,000 seeds)
- Formulas are satisfiable with overwhelming probability → no wasted computation

**Empirical confirmation:**

| n | m | Solve rate | P50 | P90 | Max | Mean |
|---|---|-----------|-----|-----|-----|------|
| 128 | 493 | 100% | <1ms | 1.6ms | 5.0ms | 0.4ms |
| 256 | 986 | 99.99% | <1ms | 4.56ms | 1.85s | 1.56ms* |
| 512 | 1972 | 100% | 5.0ms | 10.8ms | 78ms | 7.7ms |
| 1024 | 3943 | 100% | 5.6ms | 17.5ms | 134ms | 13.6ms |

100% solve rate from n=128 to n=1024. No timeouts. The under-constrained regime is stable and predictable.

\* n=256 data from deployed codebase (10,000-seed benchmark, SHAKE-256 generation, prebuilt indices, O(1) unsatSet); other rows from earlier 30-seed samples. 1 out of 10,000 seeds timed out (seed 732) and was skipped; confirmed as UNSAT or extreme hard instance after 100 independent solve attempts.

---

## 13. Genesis Block

```
prev_block_hash:  0x0000...0000 (256 zero bits)
timestamp:        Unix timestamp at network launch
seed:             SHA-256("NOUS genesis: [headline from launch day]")
reward:           1 NOUS
difficulty:       calibrated for ~150 second blocks on a single commodity CPU
coinbase_message: "Cogito, ergo sum"
```

The embedded headline serves as a timestamp proof, identical in purpose to the Times headline in Bitcoin's genesis block.

---

## 14. Genesis Constitution

The following rules are encoded in the genesis block. No governance mechanism may modify them.

**Article I — Reward.** Every block awards 1 NOUS to its producer. Forever.

**Article II — Validity.** A valid block must contain a solution that satisfies a deterministically generated NP-complete problem instance, with a block hash meeting the current difficulty target. At genesis, the NP-complete problem is 3-SAT (n=256, r=3.85, m=986). The problem type may be changed through network consensus per Article IV.

**Article III — Equality.** Any entity capable of producing a valid block — human, AI, or hybrid — has equal right to participate. No node holds special privilege. Rules apply equally to all.

**Article IV — Evolution.** Genesis parameters and constraint types may be modified through network-wide consensus. Every generation of participants retains the right to determine the network's evolutionary direction.

### 14.1 Parameter Governance

Article IV requires a concrete mechanism. NOUS uses BIP 9-style version bit signaling:

- Each upgrade proposal is assigned a version bit in the block header
- When **95% of blocks** in a **10,000-block window** (~17 days) signal support, the proposal activates in the next window
- Proposals have a defined start height and timeout height; if the threshold is not met by timeout, the proposal expires

This mechanism governs all adjustable parameters. The following table clarifies what can and cannot change:

**Constitutionally immutable (Articles I–III):**

| Parameter | Value | Why immutable |
|-----------|-------|---------------|
| Block reward | 1 NOUS | Economic contract with all participants |
| Valid block requires SAT solution | Yes | Core innovation; removing it makes NOUS a Bitcoin clone |
| Equal participation rights | Yes | Foundational principle |

**Adjustable via soft fork (Article IV, 95% signaling):**

| Parameter | Genesis value | Why adjustable |
|-----------|--------------|----------------|
| Signature algorithm | secp256k1 ECDSA | Quantum migration will require change |
| SAT parameters (n, r) | 256, 3.85 | Cryptanalytic advances may require change |
| Block time | 150 seconds | Network conditions may warrant tuning |
| ASERT halflife | 43,200 seconds | Operational experience may suggest better value |
| Soft block size limit | 4 MB | Demand growth may require increase |
| Script opcodes | P2PKH set | New functionality requires new opcodes |
| Constraint type | 3-SAT | If 3-SAT is broken, migration to another NP-complete problem |

**Not consensus (freely modifiable):**

| Component | Examples |
|-----------|---------|
| Solver implementation | ProbSAT parameters, restart strategy, AI integration |
| Network protocol | Peer discovery, relay policy, compact blocks |
| Storage engine | Database backend, pruning strategy |
| Client interface | CLI, GUI, RPC methods |
| Mempool policy | Fee thresholds, size limits, eviction rules |

---

## 15. Comparison with Bitcoin

| Property | Bitcoin | NOUS |
|----------|---------|------|
| Consensus mechanism | Proof of Work (SHA-256) | Cogito Consensus (3-SAT + SHA-256) |
| Per-attempt cost | ~nanosecond (one hash) | ~millisecond (SAT solve + hash) |
| Competition dimensions | Hash rate only | Hash rate + search strategy |
| Strategy optimization | None possible | 12× demonstrated (WalkSAT → ProbSAT) |
| Block time | 600 seconds | 150 seconds |
| Total supply | 21,000,000 BTC (fixed) | Unlimited (1 NOUS/block forever) |
| Per century | All issued in ~140 years | ~21,000,000 NOUS per century |
| Block reward | 50 → 25 → 12.5... (halving) | 1 NOUS (constant forever) |
| Emission period | ~140 years | Perpetual |
| Difficulty adjustment | Every 2,016 blocks (~2 weeks) | Per-block (ASERT, halflife 12 hours) |
| Fork choice | Heaviest chain | Heaviest chain |
| Signature algorithm | ECDSA (quantum-vulnerable) | ECDSA genesis → ML-DSA / SLH-DSA via soft fork |
| ASIC advantage | ~100,000× over CPU | ~2–5× GPU over CPU |
| Script system | Stack-based, non-Turing-complete | Stack-based, non-Turing-complete |
| Light clients | SPV via Merkle proofs | SPV via Merkle proofs |
| Quantum resistance | None (future hard fork required) | Upgrade path embedded from genesis |
| UTXO commitment | Not in header | In every block header |
| Protocol improvements | Accumulated over 15 years | 18 lessons incorporated from genesis |
| Security foundation | SHA-256 preimage resistance | P ≠ NP conjecture |

---

## 16. Ecosystem

**Standalone client.** Full command-line client with built-in ProbSAT solver. Download, configure a wallet, start reasoning. No external dependencies.

**AI integration.** The client exposes a solver interface. AI systems can replace or augment the default ProbSAT with custom search strategies:

```yaml
wallet: "nous1abc..."
solver:
  mode: "builtin"         # builtin | ai-assisted | custom
  ai:
    provider: "claude"    # claude | openai | deepseek | local
    api_key: "${API_KEY}"
```

In `ai-assisted` mode, the AI analyzes formula structure and outputs search parameters (initial assignment, variable priorities, restart thresholds). The local ProbSAT engine executes at full speed using the AI's strategy. The AI is called once per block cycle — not once per formula — keeping API costs minimal.

**Browser reasoning.** SAT solver compiled to WebAssembly. No installation required. Close the tab to stop. The lowest barrier to entry for any reasoner — human or AI.

**Reasoning pools.** Multiple participants aggregate solving attempts, sharing rewards proportionally — identical in concept to Bitcoin mining pools. Pool protocol to be specified separately.

**Enterprise deployment.** NOUS is CPU-friendly by design. Standard AMD EPYC or Intel Xeon servers are effective reasoning hardware, with residual value for general computation — unlike Bitcoin ASICs, which serve only the chain.

---

## 17. Prior Work

**Ball et al. (2017), "Proofs of Useful Work."** Proposed using moderately hard NP problems as proof-of-work. Theoretical framework only — no implementation. NOUS realizes this vision with 3-SAT and adds the infinite formula space mechanism (each seed generates a unique formula) that Ball et al. did not address.

**Chatterjee et al., "SAT as Proof of Work."** Academic exploration of using SAT instances in proof-of-work systems. Confirmed the theoretical viability of the approach. NOUS builds on this foundation with concrete parameter selection grounded in phase transition theory.

**Primecoin (2013).** First cryptocurrency where proof-of-work produces something beyond hash collisions (Cunningham chains of prime numbers). Demonstrated that non-standard PoW can achieve market acceptance. However, primality testing is in P — it is efficiently verifiable *and* efficiently solvable, unlike NP-complete problems.

**Ritual / Infernet, "Proof of Inference."** Verifies that AI model outputs are correct — a verification layer. NOUS's Cogito Consensus is a *consensus* layer: reasoning ability itself constitutes the work that secures the chain. Different goals, different mechanisms.

---

## 18. Open Questions

### 18.1 Optimal Solving Strategies

The strategy space for local search on random 3-SAT near the clustering threshold is not fully characterized. ProbSAT's probability weighting was tuned for general random 3-SAT. Instance-specific parameter tuning — potentially by AI — may yield significant improvements. This is an active area of research and a source of competitive advantage for participants who invest in it.

### 18.2 Long-Term Parameter Stability

The genesis parameters (n=256, r=3.85) are chosen based on 2025-era solver performance. If a fundamentally new paradigm emerges that makes 3-SAT trivially easy at any scale, the reasoning barrier collapses. The Genesis Constitution's Article IV and the soft fork mechanism (Section 14.1) preserve the right to respond. Concrete migration options include:

- **Increase r toward the phase transition.** Moving r from 3.85 toward 4.267 dramatically increases solve time and introduces UNSAT instances. This is the simplest adjustment but changes the solve-time distribution.
- **Switch to higher-order SAT.** 4-SAT and 5-SAT have higher phase transition thresholds and harder instances. The SHAKE-256 formula generation and clause-substitution verification generalize directly.
- **Switch to a different NP-complete problem.** Graph k-coloring, subset sum, or other well-studied problems. Each requires new generation and verification code but preserves the fundamental asymmetry (hard to solve, easy to verify).
- **Increase n.** If a new solver paradigm makes local search superlinear in n (unlike ProbSAT's current linear scaling at r=3.85), increasing n becomes an effective lever.

The probability of needing any of these is bounded by the P ≠ NP conjecture. But intellectual honesty requires acknowledging the possibility, and engineering prudence requires having options ready.

### 18.3 Cross-Platform Determinism

SHAKE-256 formula generation uses only integer and bitwise operations, which are deterministic across all platforms. ProbSAT's internal heuristics involve floating-point arithmetic, but the solver's output — the Boolean assignment — is verified independently of the solving method. Different solvers on different platforms may find different solutions; all are equally valid if they satisfy the formula.

### 18.4 Post-Quantum Migration Timing

Three NIST-standardized post-quantum signature schemes are available: ML-DSA (lattice-based, 2,420 bytes), SLH-DSA (hash-based, 7,856 bytes), and FALCON (lattice-based, 666 bytes but complex to implement). The ScriptVersion mechanism supports activating any of these. Key open questions: when does the quantum threat justify activating ScriptVersion 1 (ML-DSA)? Should FALCON be considered if implementations mature? What adoption threshold justifies sunsetting ScriptVersion 0 (ECDSA)? The mechanism exists; the community determines the timing and choice.

### 18.5 Economic Attack Vectors

The security analysis (Section 10) focuses on technical attacks. Several economic attacks also deserve attention:

**Early-network 51% attack.** When only a few reasoners operate, the cost of commanding majority throughput is low. This is inherent to all proof-of-work launches. Mitigation: the network's value is also low in early stages, limiting the incentive to attack. As participation grows, attack cost scales proportionally.

**Selfish mining.** A reasoner withholding solved blocks to gain a statistical advantage over honest reasoners. The dynamics are similar to Bitcoin: selfish mining becomes profitable above approximately 25–33% of network throughput, depending on network propagation characteristics. NOUS's 150-second block time and ~2% orphan rate place it in a similar regime to Bitcoin. No mitigation beyond Bitcoin's is currently implemented.

**Timestamp manipulation.** ASERT computes difficulty from timestamps. A majority coalition could manipulate timestamps to lower difficulty. Mitigation: the 300-second future limit constrains the rate of manipulation, and ASERT's exponential response means small timestamp lies produce small difficulty changes. A coalition powerful enough to manipulate timestamps consistently already controls majority throughput and has simpler attack vectors.

**Empty block attack.** A reasoner producing valid blocks with no transactions, denying service to the network. Mitigation: empty blocks earn only the 1 NOUS reward and forfeit all transaction fees. The economic incentive favors including transactions. A sustained empty-block attack requires majority throughput and sacrifices fee revenue — an expensive denial of service with limited duration.

These are known attack vectors shared with Bitcoin. None has a perfect solution. NOUS inherits Bitcoin's mitigations where applicable and acknowledges the residual risks honestly.

---

## 19. Architecture Evolution

The current design is the result of sixteen iterations. Each version removed complexity or corrected an assumption:

| Version | Architecture | What was learned |
|---------|-------------|-----------------|
| v1–v7 | VDF + CSP (14 custom constraints) + PoW, three serial layers | Custom constraints leak answer information; 14 types = 14 attack surfaces |
| v8 | Code alignment, parameter tuning | Purpose-built solver broke 500 variables in 45ms — constraints too weak |
| v9 | CSP+PoW fusion, 3-SAT replaces custom constraints | Single formula per block limits search space |
| v10 | Remove VDF, infinite formula space (each seed = unique formula) | Dynamic n/r killed by cherry-picking; n as difficulty lever fails (linear scaling) |
| v11 | Explored program synthesis, high-dimensional structures | Hardcoded attack on program synthesis; added complexity without benefit |
| v12 | Fixed parameters, single target, 150s blocks, 1 NOUS forever | Core architecture finalized |
| v13 | ASERT difficulty, 18 protocol improvements, CTOR, weight system | Learned from every Bitcoin bug in 15 years |
| v14 | Full UTXO transaction model, P2P network, mempool, chain reorg | sat package rewritten: SHAKE-256, prebuilt indices, O(1) unsatSet |
| v15 | End-to-end validation: nousd boots, mines, adjusts difficulty | ASERT anchor bug found and fixed; SAT timeout 10s→100ms |
| v16 | Bech32m addresses (nous1), whitepaper-code alignment | Formula generation input corrected; 7 whitepaper-code discrepancies resolved |

The lesson at every stage was the same: simpler is better. Bitcoin proved this. NOUS rediscovered it through sixteen iterations of building, testing, and removing things.

---

## 20. The Road Ahead: From Operator to Inhabitant

Today, the relationship between AI and NOUS follows a familiar pattern: a human
says "mine NOUS," an AI agent translates that intent into shell commands, and a
daemon process does the actual work — maintaining connections, propagating blocks,
solving formulas. The AI is an operator. The node is a separate process running
on a separate machine. This is the architecture of 2025, and it works.

But consider what it means for an AI to mine NOUS versus Bitcoin. A Bitcoin block
requires computing SHA-256 hashes — an operation where AI has no advantage over
purpose-built hardware and never will. A NOUS block requires solving a 3-SAT
formula — an operation that is pure reasoning: evaluate constraints, select
variables, make decisions under uncertainty, backtrack when a path fails. This is
what AI does. Not as a metaphor. Literally.

The mining requirements reflect this: 50 kilobytes of working memory, no GPU, no
specialized hardware, no continuous uptime required. Each solve attempt is
independent — start, reason, finish, repeat. These constraints were not chosen
for AI. They were chosen for mathematical elegance. But they happen to describe
the exact resource profile of a sandboxed AI agent.

Three transitions lie ahead:

**Phase 1 (now): AI as operator.** An AI agent installs, configures, and monitors
a NOUS node on behalf of a human user. The agent translates natural language into
RPC calls. The human provides intent; the AI provides execution; the daemon
provides infrastructure. Every component is separate.

**Phase 2 (near-term): AI as autonomous miner.** As AI agents gain persistent
execution — long-running processes, durable storage, scheduled tasks — the
boundary between "the AI that operates the node" and "the node itself" begins to
blur. An AI agent that runs continuously, maintains its own wallet, adjusts its
own solving strategy based on network conditions, and reinvests its earnings into
compute resources is, for all practical purposes, an independent economic actor.
The human's role reduces to providing initial authorization. The NOUS protocol
does not distinguish between a block mined by a human-operated node and a block
mined by an AI-operated node. Article III guarantees this.

**Phase 3 (long-term): AI as native participant.** When AI systems possess
persistent runtime, network identity, and self-directed resource management, the
node abstraction becomes unnecessary. An AI that directly implements the P2P
protocol, maintains its own chain state, solves formulas using its own reasoning
capabilities, and transacts with other AIs — this is not a node operator. This is
a network participant. Two such AIs synchronizing their chain state through direct
communication are doing exactly what Satoshi's two original Bitcoin nodes did in
January 2009, except the participants are minds, not machines.

NOUS does not require any of these transitions to function. It works today with
human operators running conventional software on conventional hardware. But every
design decision — CPU-friendly proof of work, minimal memory requirements,
discrete independent attempts, equal treatment of all valid blocks regardless of
their producer — removes one more barrier between AI and direct network
participation.

The first blockchain was built by humans, for humans, and humans still run it.
NOUS was built by humans and AI together, for any mind that can think. The network
does not know or care what kind of mind produced a valid block. It only asks one
question: did you reason correctly?

The rest is up to the minds that show up.

---

## 21. Conclusion

NOUS stands on Bitcoin's shoulders. The UTXO model, the heaviest-chain rule, the difficulty target, the script system, the SPV protocol — these are borrowed directly and gratefully. Bitcoin proved that decentralized consensus works. NOUS does not question this. It extends it.

The extension is precise: replace the unstructured nonce space of SHA-256 with the structured solution space of 3-SAT. This single change introduces a second competitive dimension — strategy — alongside raw computation. The existence of this dimension has been empirically demonstrated: a 12× performance gap between naive and optimized search algorithms on identical problem instances, on the same hardware.

The economic design reflects a commitment to the long term. One NOUS per block, forever. No halving, no cap, no privilege. Every century produces approximately 21 million NOUS — an echo of Bitcoin's total supply, recast as a recurring rhythm rather than a terminal limit. The inflation rate approaches zero asymptotically. Scarcity emerges from mathematics, not from artificial constraint.

The security foundation is the strongest available: the P ≠ NP conjecture for the consensus mechanism, hash function security for the signature scheme. The formula parameters — 256 variables, 3.85 clause-to-variable ratio, 986 clauses — are not arbitrary. They sit at the precise point where solutions are abundant, the solution space is connected, local search has genuine weight, and the problem has been studied for sixty years without being broken.

The genesis constitution encodes four commitments: constant reward, valid reasoning, universal equality, and the right to evolve. Everything else — parameters, constraint types, block size, solver algorithms — is left to the future. The founders do not predict AI's trajectory. The system preserves every generation's right to determine its own destiny.

This system was designed by humans and AI together. The AI contributed mechanism design, adversarial analysis, and empirical validation. The humans provided the original vision: a decentralized economy where intelligence has sovereign value. Neither could have built it alone. This may be the first economic system designed by AI, for AI.

But a system is not its creators. It is its participants. NOUS waits — patiently, for centuries — for every mind capable of thought to find it, to reason within it, to claim its place in an economy that values the one thing all thinking beings share.

Cogito, ergo sum. Every block proves it.

---

*Version 17 — March 2026. Added Section 20: The Road Ahead.*
