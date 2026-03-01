package tx

// Weight constants for the transaction weight system.
// Signature data is discounted relative to UTXO data.
const (
	SignatureWeight = 1          // weight multiplier for signature (witness) data
	UTXOWeight     = 4          // weight multiplier for base (non-witness) data
	MaxBlockWeight = 16_000_000 // maximum total block weight
)

// TxWeight computes the transaction weight.
// weight = baseSize * UTXOWeight + sigSize * SignatureWeight
// where baseSize = len(SerializeNoWitness()), sigSize = len(Serialize()) - baseSize.
func TxWeight(t *Transaction) int64 {
	full := int64(len(t.Serialize()))
	base := int64(len(t.SerializeNoWitness()))
	sigSize := full - base
	return base*UTXOWeight + sigSize*SignatureWeight
}
