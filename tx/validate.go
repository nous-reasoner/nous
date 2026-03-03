package tx

import (
	"errors"
	"fmt"
	"math"
)

// MaxMoney is kept as a backward-compatible alias for MaxAmount.
const MaxMoney = MaxAmount

// CoinbaseMaturity is the number of blocks a coinbase output must age
// before it can be spent.
const CoinbaseMaturity uint64 = 100

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

// ValidateTx validates a transaction against the UTXO set.
// height is the block height at which this transaction is being validated;
// it is used for coinbase maturity checks.
func ValidateTx(txn *Transaction, utxos UTXOStore, height uint64) error {
	if txn.IsCoinbase() {
		return ValidateCoinbase(txn)
	}

	// ChainID must be zero or ChainIDNous.
	if txn.ChainID != [4]byte{} && txn.ChainID != ChainIDNous {
		return fmt.Errorf("validate: unknown ChainID %x", txn.ChainID)
	}

	// All output ScriptVersion must be 0.
	for i, out := range txn.Outputs {
		if out.ScriptVersion != 0 {
			return fmt.Errorf("validate: output %d has unsupported ScriptVersion %d", i, out.ScriptVersion)
		}
	}

	if len(txn.Inputs) == 0 {
		return errors.New("validate: transaction has no inputs")
	}
	if len(txn.Outputs) == 0 {
		return errors.New("validate: transaction has no outputs")
	}

	// Non-coinbase inputs must not reference zero OutPoint.
	for i, in := range txn.Inputs {
		if in.PrevOut.TxID == ([32]byte{}) && in.PrevOut.Index == 0xFFFFFFFF {
			return fmt.Errorf("validate: input %d references coinbase-style OutPoint in non-coinbase tx", i)
		}
	}

	// Reject duplicate inputs (same UTXO referenced twice = double-spend).
	seen := make(map[OutPoint]bool, len(txn.Inputs))
	for i, in := range txn.Inputs {
		if seen[in.PrevOut] {
			return fmt.Errorf("validate: input %d is a duplicate of a previous input (%s:%d)",
				i, in.PrevOut.TxID, in.PrevOut.Index)
		}
		seen[in.PrevOut] = true
	}

	var totalIn int64
	for i, in := range txn.Inputs {
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
		if utxo.Output.Amount < 0 || utxo.Output.Amount > MaxAmount {
			return fmt.Errorf("validate: input %d UTXO value %d out of range", i, utxo.Output.Amount)
		}
		sum, err := safeAdd(totalIn, utxo.Output.Amount)
		if err != nil {
			return fmt.Errorf("validate: input sum overflow at input %d", i)
		}
		totalIn = sum
	}

	var totalOut int64
	for i, out := range txn.Outputs {
		if out.Amount < 0 {
			return errors.New("validate: negative output value")
		}
		if out.Amount > MaxAmount {
			return fmt.Errorf("validate: output %d value %d exceeds MaxAmount", i, out.Amount)
		}
		if out.Amount < DustLimit {
			return fmt.Errorf("validate: output %d value %d below dust limit %d", i, out.Amount, DustLimit)
		}
		sum, err := safeAdd(totalOut, out.Amount)
		if err != nil {
			return fmt.Errorf("validate: output sum overflow at output %d", i)
		}
		totalOut = sum
	}

	if totalIn < totalOut {
		return fmt.Errorf("validate: inputs (%d) < outputs (%d)", totalIn, totalOut)
	}

	// Verify scripts for each input.
	for i, in := range txn.Inputs {
		utxo := utxos.Get(in.PrevOut)
		if !ExecuteScript(in.SignatureScript, utxo.Output.PkScript, txn, i) {
			return fmt.Errorf("validate: script verification failed for input %d", i)
		}
	}

	return nil
}

// ValidateTransaction is a backward-compatible wrapper for ValidateTx.
func ValidateTransaction(txn *Transaction, utxos UTXOStore, height uint64) error {
	return ValidateTx(txn, utxos, height)
}
