package tx

import (
	"errors"
	"fmt"
	"math"
)

// MaxMoney is the maximum number of nou that can ever exist.
// 21,000,000,000 NOUS × 100,000,000 nou/NOUS = 2,100,000,000,000,000,000 nou.
const MaxMoney int64 = 21_000_000_000_00000000

// CoinbaseMaturity is the number of blocks a coinbase output must age
// before it can be spent.
const CoinbaseMaturity uint64 = 100

// DustLimit is the minimum output value (in nou) for non-coinbase transactions.
// Outputs below this threshold bloat the UTXO set without economic purpose.
const DustLimit int64 = 546

// safeAdd returns a + b or an error if the result would overflow int64.
func safeAdd(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, errors.New("integer overflow")
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, errors.New("integer underflow")
	}
	return a + b, nil
}

// ValidateTransaction validates a transaction against the UTXO set.
// height is the block height at which this transaction is being validated;
// it is used for coinbase maturity checks.
func ValidateTransaction(tx *Transaction, utxos *UTXOSet, height uint64) error {
	if tx.IsCoinbase() {
		return validateCoinbase(tx)
	}

	if len(tx.Inputs) == 0 {
		return errors.New("validate: transaction has no inputs")
	}
	if len(tx.Outputs) == 0 {
		return errors.New("validate: transaction has no outputs")
	}

	// Reject duplicate inputs (same UTXO referenced twice = double-spend).
	seen := make(map[OutPoint]bool, len(tx.Inputs))
	for i, in := range tx.Inputs {
		if seen[in.PrevOut] {
			return fmt.Errorf("validate: input %d is a duplicate of a previous input (%s:%d)",
				i, in.PrevOut.TxID, in.PrevOut.Index)
		}
		seen[in.PrevOut] = true
	}

	var totalIn int64
	for i, in := range tx.Inputs {
		utxo := utxos.Get(in.PrevOut)
		if utxo == nil {
			return fmt.Errorf("validate: input %d references missing UTXO %s:%d",
				i, in.PrevOut.TxID, in.PrevOut.Index)
		}
		// Coinbase maturity: coinbase outputs cannot be spent until
		// CoinbaseMaturity blocks have passed.
		if utxo.IsCoinbase && height < utxo.Height+CoinbaseMaturity {
			return fmt.Errorf("validate: input %d spends immature coinbase (created at height %d, current %d, need %d)",
				i, utxo.Height, height, utxo.Height+CoinbaseMaturity)
		}
		if utxo.Output.Value < 0 || utxo.Output.Value > MaxMoney {
			return fmt.Errorf("validate: input %d UTXO value %d out of range", i, utxo.Output.Value)
		}
		sum, err := safeAdd(totalIn, utxo.Output.Value)
		if err != nil {
			return fmt.Errorf("validate: input sum overflow at input %d", i)
		}
		totalIn = sum
	}

	var totalOut int64
	for i, out := range tx.Outputs {
		if out.Value < 0 {
			return errors.New("validate: negative output value")
		}
		if out.Value > MaxMoney {
			return fmt.Errorf("validate: output %d value %d exceeds MaxMoney", i, out.Value)
		}
		if out.Value < DustLimit {
			return fmt.Errorf("validate: output %d value %d below dust limit %d", i, out.Value, DustLimit)
		}
		sum, err := safeAdd(totalOut, out.Value)
		if err != nil {
			return fmt.Errorf("validate: output sum overflow at output %d", i)
		}
		totalOut = sum
	}

	if totalIn < totalOut {
		return fmt.Errorf("validate: inputs (%d) < outputs (%d)", totalIn, totalOut)
	}

	// Verify scripts for each input.
	for i, in := range tx.Inputs {
		utxo := utxos.Get(in.PrevOut)
		if !ExecuteScript(in.ScriptSig, utxo.Output.ScriptPubKey, tx, i) {
			return fmt.Errorf("validate: script verification failed for input %d", i)
		}
	}

	return nil
}

func validateCoinbase(tx *Transaction) error {
	if len(tx.Inputs) != 1 {
		return errors.New("validate: coinbase must have exactly 1 input")
	}
	if tx.Inputs[0].PrevOut.TxID != ([32]byte{}) {
		return errors.New("validate: coinbase input must reference zero hash")
	}
	if tx.Inputs[0].PrevOut.Index != 0xFFFFFFFF {
		return errors.New("validate: coinbase input index must be 0xFFFFFFFF")
	}
	if len(tx.Outputs) == 0 {
		return errors.New("validate: coinbase must have at least 1 output")
	}
	return nil
}
