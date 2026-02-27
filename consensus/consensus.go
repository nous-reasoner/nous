// Package consensus implements the NOUS consensus engine.
//
// The consensus mechanism combines three layers:
//   - VDF (Verifiable Delay Function) for time-gating
//   - CSP (Constraint Satisfaction Problem) for AI-proof-of-work
//   - PoW (Proof of Work) for final difficulty tuning
//
// Mining flow (mine.go):
//  1. Compute VDF(prevBlockHash, minerPubKey)
//  2. Generate CSP from VDF output seed
//  3. Solve CSP using AI model (brute-force placeholder)
//  4. Find nonce such that Hash(header) < target
//
// Validation flow (validate.go):
//  1. Verify VDF proof (O(log T))
//  2. Re-derive CSP from VDF output
//  3. Check CSP solution against constraints (no AI needed)
//  4. Verify PoW hash < target
//  5. Verify all transactions and UTXO consistency
//
// Difficulty adjustment (adjust.go):
//   Three-layer independent adjustment every 144 blocks,
//   with ±25% cap (normal) and -50% emergency cap.
//
// Chain state (chain.go):
//   Tracks tip, UTXO set, and current difficulty parameters.
package consensus
