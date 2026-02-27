// Package vdf implements the Wesolowski Verifiable Delay Function.
//
// The VDF guarantees that each miner must spend a minimum sequential
// computation time before participating in block production, regardless
// of parallel resources.
//
// Scheme (Wesolowski 2019):
//   Evaluate: y = g^(2^T) mod N  (T sequential squarings, not parallelizable)
//   Proof:    π = g^(⌊2^T / l⌋) mod N  where l = HashToPrime(g, y)
//   Verify:   π^l · g^r ≡ y (mod N)    where r = 2^T mod l
//
// Verification requires only ~O(log T) modular exponentiations (milliseconds).
package vdf

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/nous-chain/nous/crypto"
)

// defaultModulus is the RSA-2048 challenge number — a well-known, publicly
// generated, unfactored modulus. Using this avoids a trusted setup ceremony.
var defaultModulus *big.Int

func init() {
	var ok bool
	defaultModulus, ok = new(big.Int).SetString(
		"2519590847565789349402718324004839857142928212620403202777713783"+
			"6043662020707595556264018525880784406918290641249515082189298559"+
			"1491761845028084891200728449926873928072877767359714183472702618"+
			"9637501497182469116507761337985909570009733045974880842840179742"+
			"9100642458691817195118746121515172654632282216869987549182422433"+
			"6372590851418654620435767984233871847744479207399342365848238242"+
			"8119816381501067481045166037730605620161967625613384414360331195"+
			"1871871250550905960895752507840966021178257831834443415038706547"+
			"7955100195051559704570798433101348958758043730139148775455046945"+
			"0148765504076515075258910568592989572336266079134525652942975487"+
			"7808909245429557665254710637437815415237516448053809", 10)
	if !ok {
		panic("vdf: failed to parse RSA-2048 modulus")
	}
}

// Params holds the public parameters for the VDF.
type Params struct {
	N *big.Int // RSA modulus
	T uint64   // number of sequential squarings
}

// Output contains the VDF evaluation result and Wesolowski proof.
type Output struct {
	Y     []byte // g^(2^T) mod N, big-endian, left-padded to modulus byte length
	Proof []byte // Wesolowski proof π, same encoding
}

// DefaultParams returns VDF parameters with the RSA-2048 modulus and default T.
func DefaultParams() *Params {
	return &Params{
		N: new(big.Int).Set(defaultModulus),
		T: 1 << 20,
	}
}

// NewParams creates VDF parameters with a custom T and the default modulus.
func NewParams(t uint64) *Params {
	return &Params{
		N: new(big.Int).Set(defaultModulus),
		T: t,
	}
}

// MakeInput computes the per-miner VDF input: SHA256(prevBlockHash || compressedPubKey).
// Each miner gets a unique input because their public key differs.
func MakeInput(prevBlockHash crypto.Hash, minerPubKey *crypto.PublicKey) []byte {
	buf := make([]byte, 32+33)
	copy(buf[:32], prevBlockHash[:])
	copy(buf[32:], minerPubKey.SerializeCompressed())
	h := crypto.Sha256(buf)
	return h[:]
}

// Evaluate computes the Wesolowski VDF and returns the output with proof.
//
// Phase 1 (T squarings):  y = g^(2^T) mod N
// Phase 2 (T steps):      π = g^⌊2^T/l⌋ mod N  via long-division trick
func Evaluate(params *Params, input []byte) (*Output, error) {
	if params.N == nil || params.N.Sign() <= 0 {
		return nil, errors.New("vdf: invalid modulus")
	}
	if params.T == 0 {
		return nil, errors.New("vdf: T must be > 0")
	}

	N := params.N
	T := params.T
	g := inputToGroup(input, N)
	modLen := (N.BitLen() + 7) / 8

	// Phase 1: y = g^(2^T) mod N — T sequential squarings
	y := new(big.Int).Set(g)
	for i := uint64(0); i < T; i++ {
		y.Mul(y, y)
		y.Mod(y, N)
	}

	// Derive challenge prime l = HashToPrime(g, y)
	l := hashToPrime(g, y)

	// Phase 2: π = g^⌊2^T/l⌋ mod N via long-division
	//
	// We track r = (2^(i+1)) mod l alongside pi.
	// Whenever r overflows l, we have a '1' bit in the quotient
	// and multiply pi by g accordingly.
	pi := new(big.Int).SetInt64(1)
	r := new(big.Int).SetInt64(1)
	two := big.NewInt(2)

	for i := uint64(0); i < T; i++ {
		r.Mul(r, two) // r = 2·r
		pi.Mul(pi, pi)
		pi.Mod(pi, N) // pi = pi²  mod N
		if r.Cmp(l) >= 0 {
			r.Sub(r, l)    // r = r - l
			pi.Mul(pi, g)  // pi = pi·g
			pi.Mod(pi, N)
		}
	}

	return &Output{
		Y:     padBigInt(y, modLen),
		Proof: padBigInt(pi, modLen),
	}, nil
}

// Verify checks the Wesolowski VDF proof in fast time:
//
//  1. Recompute g from the input
//  2. l = HashToPrime(g, y)
//  3. r = 2^T mod l          (fast: modular exponentiation, O(log T) multiplications)
//  4. Accept iff π^l · g^r ≡ y (mod N)
func Verify(params *Params, input []byte, output *Output) bool {
	if params.N == nil || output == nil || len(output.Y) == 0 || len(output.Proof) == 0 {
		return false
	}

	N := params.N
	g := inputToGroup(input, N)
	y := new(big.Int).SetBytes(output.Y)
	pi := new(big.Int).SetBytes(output.Proof)

	// Range check
	if y.Sign() <= 0 || y.Cmp(N) >= 0 {
		return false
	}
	if pi.Sign() <= 0 || pi.Cmp(N) >= 0 {
		return false
	}

	l := hashToPrime(g, y)

	// r = 2^T mod l
	tBig := new(big.Int).SetUint64(params.T)
	r := new(big.Int).Exp(big.NewInt(2), tBig, l)

	// Check: π^l · g^r ≡ y  (mod N)
	piL := new(big.Int).Exp(pi, l, N)
	gR := new(big.Int).Exp(g, r, N)
	result := new(big.Int).Mul(piL, gR)
	result.Mod(result, N)

	return result.Cmp(y) == 0
}

// OutputHash returns SHA-256(Y) for embedding in the block header (32 bytes).
func (o *Output) OutputHash() crypto.Hash {
	return crypto.Sha256(o.Y)
}

// ProofHash returns SHA-256(Proof) for embedding in the block header (32 bytes).
func (o *Output) ProofHash() crypto.Hash {
	return crypto.Sha256(o.Proof)
}

// --- internal helpers ---

// inputToGroup maps arbitrary input bytes to an element g ∈ [2, N-1]
// by hash-expanding the input to the modulus bit length.
func inputToGroup(input []byte, N *big.Int) *big.Int {
	nBytes := (N.BitLen() + 7) / 8

	// Expand input to nBytes using SHA-256 in counter mode
	expanded := make([]byte, 0, nBytes+sha256.Size)
	buf := make([]byte, len(input)+4)
	copy(buf, input)
	for i := uint32(0); len(expanded) < nBytes; i++ {
		binary.BigEndian.PutUint32(buf[len(input):], i)
		h := sha256.Sum256(buf)
		expanded = append(expanded, h[:]...)
	}

	g := new(big.Int).SetBytes(expanded[:nBytes])
	g.Mod(g, N)

	// g must be >= 2 to avoid trivial fixed points (0^x=0, 1^x=1)
	if g.Cmp(big.NewInt(2)) < 0 {
		g.SetInt64(2)
	}
	return g
}

// hashToPrime derives a prime l from (g, y) by hashing and scanning forward.
// l ≈ 256 bits, so verification exponents are manageable.
func hashToPrime(g, y *big.Int) *big.Int {
	data := append(g.Bytes(), y.Bytes()...)
	h := sha256.Sum256(data)
	candidate := new(big.Int).SetBytes(h[:])

	// Ensure odd
	candidate.SetBit(candidate, 0, 1)

	// Scan forward for the next prime (20-round Miller-Rabin)
	for !candidate.ProbablyPrime(20) {
		candidate.Add(candidate, big.NewInt(2))
	}
	return candidate
}

// padBigInt returns x as big-endian bytes left-padded to exactly length bytes.
func padBigInt(x *big.Int, length int) []byte {
	b := x.Bytes()
	if len(b) >= length {
		return b[len(b)-length:]
	}
	padded := make([]byte, length)
	copy(padded[length-len(b):], b)
	return padded
}
