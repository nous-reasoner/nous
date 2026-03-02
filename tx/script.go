package tx

import (
	"bytes"

	"nous/crypto"
)

// Script opcodes for P2PKH.
const (
	OpDup         = 0x76
	OpHash160     = 0xa9
	OpEqualVerify = 0x88
	OpCheckSig    = 0xac
)

// CreateP2PKHLockScript builds a standard P2PKH locking script:
// OP_DUP OP_HASH160 <20> <pubKeyHash> OP_EQUALVERIFY OP_CHECKSIG
func CreateP2PKHLockScript(pubKeyHash []byte) []byte {
	script := make([]byte, 0, 25)
	script = append(script, OpDup, OpHash160, byte(len(pubKeyHash)))
	script = append(script, pubKeyHash...)
	script = append(script, OpEqualVerify, OpCheckSig)
	return script
}

// CreateP2PKHUnlockScript builds a standard P2PKH unlocking script:
// <len(sig)> <sig> <len(pubKey)> <pubKey>
func CreateP2PKHUnlockScript(sig []byte, pubKey []byte) []byte {
	script := make([]byte, 0, 1+len(sig)+1+len(pubKey))
	script = append(script, byte(len(sig)))
	script = append(script, sig...)
	script = append(script, byte(len(pubKey)))
	script = append(script, pubKey...)
	return script
}

// ExtractPubKeyHashFromP2PKH extracts the 20-byte public key hash from a
// standard P2PKH script. Returns nil if the script is not a valid P2PKH script.
func ExtractPubKeyHashFromP2PKH(script []byte) []byte {
	// OP_DUP OP_HASH160 0x14 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
	if len(script) == 25 &&
		script[0] == OpDup &&
		script[1] == OpHash160 &&
		script[2] == 20 &&
		script[23] == OpEqualVerify &&
		script[24] == OpCheckSig {
		hash := make([]byte, 20)
		copy(hash, script[3:23])
		return hash
	}
	return nil
}

// ExecuteScript runs a P2PKH script engine. It first executes scriptSig
// (the unlock script), then executes scriptPubKey (the lock script) on
// the resulting stack. Returns true if the script succeeds.
func ExecuteScript(scriptSig, scriptPubKey []byte, txn *Transaction, inputIndex int) bool {
	var stack [][]byte

	// Execute scriptSig (unlock).
	if !execute(scriptSig, &stack, txn, inputIndex, scriptPubKey) {
		return false
	}

	// Execute scriptPubKey (lock) on the same stack.
	if !execute(scriptPubKey, &stack, txn, inputIndex, scriptPubKey) {
		return false
	}

	// Script succeeds if the top of the stack is non-zero.
	if len(stack) == 0 {
		return false
	}
	return isTrue(stack[len(stack)-1])
}

func execute(script []byte, stack *[][]byte, txn *Transaction, inputIndex int, subscript []byte) bool {
	r := bytes.NewReader(script)
	for r.Len() > 0 {
		op, err := r.ReadByte()
		if err != nil {
			return false
		}

		switch {
		case op >= 0x01 && op <= 0x4b:
			// Push data: op bytes follow.
			data := make([]byte, op)
			if _, err := r.Read(data); err != nil {
				return false
			}
			*stack = append(*stack, data)

		case op == OpDup:
			if len(*stack) < 1 {
				return false
			}
			top := (*stack)[len(*stack)-1]
			dup := make([]byte, len(top))
			copy(dup, top)
			*stack = append(*stack, dup)

		case op == OpHash160:
			if len(*stack) < 1 {
				return false
			}
			top := pop(stack)
			*stack = append(*stack, crypto.Hash160(top))

		case op == OpEqualVerify:
			if len(*stack) < 2 {
				return false
			}
			a := pop(stack)
			b := pop(stack)
			if !bytes.Equal(a, b) {
				return false
			}

		case op == OpCheckSig:
			if len(*stack) < 2 {
				return false
			}
			pubKeyBytes := pop(stack)
			sigBytes := pop(stack)

			pubKey, err := crypto.ParsePublicKey(pubKeyBytes)
			if err != nil {
				*stack = append(*stack, []byte{0})
				return true
			}
			sig, err := crypto.SignatureFromBytes(sigBytes)
			if err != nil {
				*stack = append(*stack, []byte{0})
				return true
			}

			// Compute the sighash using the scriptPubKey of the output being spent,
			// passed in as subscript.
			sigHash := txn.SigHash(inputIndex, subscript)

			if crypto.Verify(pubKey, sigHash, sig) {
				*stack = append(*stack, []byte{1})
			} else {
				*stack = append(*stack, []byte{0})
			}

		default:
			// Unknown opcode — fail.
			return false
		}
	}
	return true
}

func pop(stack *[][]byte) []byte {
	s := *stack
	top := s[len(s)-1]
	*stack = s[:len(s)-1]
	return top
}

func isTrue(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return true
		}
	}
	return false
}
