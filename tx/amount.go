package tx

import (
	"errors"
	"fmt"
	"math"
)

// Amount is a type alias for int64 representing a value in nou.
// 1 NOUS = 1e8 nou.
type Amount = int64

// Amount constants.
// NOUS uses perpetual linear emission (1 NOUS/block forever) with no supply cap.
// MaxAmount is not a supply limit — it exists solely to prevent integer overflow
// in summation and validation logic.
const (
	Coin      Amount = 1_0000_0000          // 1 NOUS in nou
	MaxAmount Amount = math.MaxInt64 / 2    // overflow guard, not a supply cap
	DustLimit Amount = 546                  // minimum non-coinbase output value
)

// CheckAmount validates that a is in the range [0, MaxAmount].
func CheckAmount(a Amount) error {
	if a < 0 {
		return fmt.Errorf("amount: negative value %d", a)
	}
	if a > MaxAmount {
		return fmt.Errorf("amount: %d exceeds max %d", a, MaxAmount)
	}
	return nil
}

// SumOutputs returns the overflow-safe sum of all output amounts.
func SumOutputs(outs []TxOut) (Amount, error) {
	var total int64
	for i, out := range outs {
		if out.Amount < 0 {
			return 0, fmt.Errorf("amount: output %d has negative value %d", i, out.Amount)
		}
		if out.Amount > MaxAmount {
			return 0, fmt.Errorf("amount: output %d value %d exceeds max", i, out.Amount)
		}
		if total > math.MaxInt64-out.Amount {
			return 0, errors.New("amount: output sum overflow")
		}
		total += out.Amount
		if total > MaxAmount {
			return 0, fmt.Errorf("amount: output sum %d exceeds max %d", total, MaxAmount)
		}
	}
	return total, nil
}
