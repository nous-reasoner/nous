// Package consensus implements the NOUS Cogito Consensus engine.
//
// The consensus mechanism uses:
//   - 3-SAT: formula generation and solving (proof of reasoning)
//   - SHA-256: block hashing for proof of work
//   - PoW target: difficulty-adjusted hash target
//
// Mining flow (mine.go):
//  1. For each seed value, generate 3-SAT formula
//  2. Solve formula using ProbSAT
//  3. Check if block hash < target
//
// Validation flow (validate.go):
//  1. Regenerate 3-SAT formula from seed
//  2. Verify SAT solution satisfies all clauses
//  3. Verify solution hash matches header
//  4. Verify PoW hash < target
//  5. Verify all transactions and UTXO consistency
//
// Difficulty adjustment (adjust.go):
//   PoW target adjustment every 1008 blocks,
//   with ±25% cap (normal) and 2x emergency cap (ratio >= 4).
//
// Chain state (chain.go):
//   Tracks tip, UTXO set, and current difficulty parameters.
package consensus
