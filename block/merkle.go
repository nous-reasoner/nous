package block

import "nous/crypto"

// ComputeMerkleRoot builds a Merkle tree from a list of transaction hashes
// and returns the root hash.
//
// Algorithm (Bitcoin-style):
//   - If the list is empty, return the zero hash.
//   - If the list has one element, that element is the root.
//   - If the number of hashes at any level is odd, duplicate the last hash.
//   - Combine consecutive pairs with DoubleSha256(left || right).
//   - Repeat until one hash remains.
func ComputeMerkleRoot(hashes []crypto.Hash) crypto.Hash {
	if len(hashes) == 0 {
		return crypto.Hash{}
	}

	// Work on a copy to avoid mutating the caller's slice.
	level := make([]crypto.Hash, len(hashes))
	copy(level, hashes)

	for len(level) > 1 {
		// If odd number, duplicate the last element.
		if len(level)%2 != 0 {
			level = append(level, level[len(level)-1])
		}

		next := make([]crypto.Hash, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			var combined [64]byte
			copy(combined[:32], level[i][:])
			copy(combined[32:], level[i+1][:])
			next[i/2] = crypto.DoubleSha256(combined[:])
		}
		level = next
	}

	return level[0]
}
