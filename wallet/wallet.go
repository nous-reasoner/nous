// Package wallet provides key management, persistence, and transaction construction.
package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"

	"nous/crypto"
	"nous/tx"
	"golang.org/x/crypto/scrypt"
)

// scrypt parameters for key derivation.
const (
	scryptN      = 32768
	scryptR      = 8
	scryptP      = 1
	scryptKeyLen = 32
	saltLen      = 32
)

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

// KeyPair holds a single key pair and its derived address.
type KeyPair struct {
	PrivateKey *crypto.PrivateKey
	PublicKey  *crypto.PublicKey
	Address    crypto.Address
}

// Wallet manages a pool of key pairs and constructs transactions.
type Wallet struct {
	Keys      []KeyPair
	Primary   int  // index of the active key pair
	IsTestnet bool // true for testnet, false for mainnet
}

// NewWallet generates a new wallet with a single fresh key pair.
func NewWallet() (*Wallet, error) {
	kp, err := generateKeyPair()
	if err != nil {
		return nil, err
	}
	return &Wallet{Keys: []KeyPair{kp}, Primary: 0}, nil
}

// GenerateNewKey adds a new key pair to the pool and returns its index.
func (w *Wallet) GenerateNewKey() (int, error) {
	kp, err := generateKeyPair()
	if err != nil {
		return 0, err
	}
	w.Keys = append(w.Keys, kp)
	return len(w.Keys) - 1, nil
}

// ImportPrivateKey imports a 32-byte private key, derives the public key
// and address, and adds it to the key pool. Returns the index.
func (w *Wallet) ImportPrivateKey(privBytes []byte) (int, error) {
	privKey, err := crypto.PrivateKeyFromBytes(privBytes)
	if err != nil {
		return 0, fmt.Errorf("wallet: import: %w", err)
	}
	pubKey := privKey.PubKey()
	addr := crypto.Address(crypto.PubKeyToBech32mAddress(pubKey))
	w.Keys = append(w.Keys, KeyPair{
		PrivateKey: privKey,
		PublicKey:  pubKey,
		Address:    addr,
	})
	return len(w.Keys) - 1, nil
}

// ExportPrivateKey returns the 32-byte serialized primary private key.
func (w *Wallet) ExportPrivateKey() []byte {
	return w.Keys[w.Primary].PrivateKey.Bytes()
}

// GetAddress returns the primary key's address.
func (w *Wallet) GetAddress() crypto.Address {
	return w.Keys[w.Primary].Address
}

// PubKeyHash returns the 20-byte public key hash for the primary key.
func (w *Wallet) PubKeyHash() []byte {
	return crypto.Hash160(w.Keys[w.Primary].PublicKey.SerializeCompressed())
}

// --- Persistence ---

// SaveToFile encrypts all private keys with AES-256-GCM and writes to path.
// The encryption key is derived from password via scrypt.
// File format: salt(32) || nonce(12) || ciphertext.
func (w *Wallet) SaveToFile(path, password string) error {
	// Serialize: primary(4 LE) + numKeys(4 LE) + [32-byte privkey]*n
	numKeys := len(w.Keys)
	plain := make([]byte, 8+numKeys*32)
	binary.LittleEndian.PutUint32(plain[0:4], uint32(w.Primary))
	binary.LittleEndian.PutUint32(plain[4:8], uint32(numKeys))
	for i, kp := range w.Keys {
		copy(plain[8+i*32:8+(i+1)*32], kp.PrivateKey.Bytes())
	}

	// Derive encryption key.
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("wallet: generate salt: %w", err)
	}
	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return fmt.Errorf("wallet: scrypt: %w", err)
	}

	// Encrypt with AES-256-GCM.
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("wallet: aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("wallet: gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("wallet: generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plain, nil)

	// Write: salt + nonce + ciphertext.
	out := make([]byte, 0, saltLen+len(nonce)+len(ciphertext))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return os.WriteFile(path, out, 0600)
}

// LoadFromFile reads an encrypted wallet file and decrypts it with password.
func LoadFromFile(path, password string) (*Wallet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wallet: read file: %w", err)
	}

	if len(data) < saltLen+12 {
		return nil, errors.New("wallet: file too short")
	}

	salt := data[:saltLen]
	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return nil, fmt.Errorf("wallet: scrypt: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("wallet: aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("wallet: gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < saltLen+nonceSize {
		return nil, errors.New("wallet: file too short for nonce")
	}
	nonce := data[saltLen : saltLen+nonceSize]
	ciphertext := data[saltLen+nonceSize:]

	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("wallet: decrypt: %w", err)
	}

	if len(plain) < 8 {
		return nil, errors.New("wallet: invalid plaintext")
	}
	primary := int(binary.LittleEndian.Uint32(plain[0:4]))
	numKeys := int(binary.LittleEndian.Uint32(plain[4:8]))
	if len(plain) != 8+numKeys*32 {
		return nil, errors.New("wallet: plaintext size mismatch")
	}

	w := &Wallet{Keys: make([]KeyPair, numKeys), Primary: primary}
	for i := 0; i < numKeys; i++ {
		privBytes := plain[8+i*32 : 8+(i+1)*32]
		privKey, err := crypto.PrivateKeyFromBytes(privBytes)
		if err != nil {
			return nil, fmt.Errorf("wallet: restore key %d: %w", i, err)
		}
		pubKey := privKey.PubKey()
		w.Keys[i] = KeyPair{
			PrivateKey: privKey,
			PublicKey:  pubKey,
			Address:    crypto.Address(crypto.PubKeyToBech32mAddress(pubKey)),
		}
	}
	return w, nil
}

// --- Balance & UTXO queries ---

// GetBalance returns the total balance across all keys in the wallet.
func (w *Wallet) GetBalance(utxoSet tx.UTXOStore) int64 {
	var total int64
	for _, kp := range w.Keys {
		pkh := crypto.Hash160(kp.PublicKey.SerializeCompressed())
		bal := utxoSet.GetBalance(pkh)
		sum, err := safeAdd(total, bal)
		if err != nil {
			return tx.MaxMoney // cap at max if overflow
		}
		total = sum
	}
	return total
}

// GetUTXOs returns all unspent outpoints belonging to any key in the wallet.
func (w *Wallet) GetUTXOs(utxoSet tx.UTXOStore) []tx.OutPoint {
	var result []tx.OutPoint
	for _, kp := range w.Keys {
		pkh := crypto.Hash160(kp.PublicKey.SerializeCompressed())
		utxos := utxoSet.FindByPubKeyHash(pkh)
		for _, u := range utxos {
			result = append(result, u.OutPoint)
		}
	}
	return result
}

// --- Transaction construction ---

// CreateTransaction builds a signed transaction sending amount to the given
// address, with the specified fee. UTXOs are selected largest-first.
// Change (if any) is sent back to the wallet's primary address.
func (w *Wallet) CreateTransaction(to crypto.Address, amount, fee int64, utxoSet tx.UTXOStore, heights ...uint64) (*tx.Transaction, error) {
	// Optional height parameter for filtering immature coinbase UTXOs.
	var currentHeight uint64
	if len(heights) > 0 {
		currentHeight = heights[0]
	}
	if amount <= 0 {
		return nil, errors.New("wallet: amount must be positive")
	}
	if fee < 0 {
		return nil, errors.New("wallet: fee must be non-negative")
	}

	needed := amount + fee

	// Decode destination pubkey hash.
	toPKH, err := crypto.DecodePubKeyHash(string(to))
	if err != nil {
		return nil, fmt.Errorf("wallet: invalid destination address: %w", err)
	}

	// Collect all wallet UTXOs with their owning key index.
	type ownedUTXO struct {
		utxo     *tx.UTXO
		keyIndex int
	}
	var available []ownedUTXO
	for ki, kp := range w.Keys {
		pkh := crypto.Hash160(kp.PublicKey.SerializeCompressed())
		utxos := utxoSet.FindByPubKeyHash(pkh)
		for _, u := range utxos {
			// Skip immature coinbase outputs.
			if currentHeight > 0 && u.IsCoinbase && currentHeight < u.Height+tx.CoinbaseMaturityFor(w.IsTestnet) {
				continue
			}
			available = append(available, ownedUTXO{utxo: u, keyIndex: ki})
		}
	}

	// Sort by value descending (largest first).
	sort.Slice(available, func(i, j int) bool {
		return available[i].utxo.Output.Amount > available[j].utxo.Output.Amount
	})

	// Select inputs.
	var selected []ownedUTXO
	var totalIn int64
	for _, ou := range available {
		selected = append(selected, ou)
		sum, err := safeAdd(totalIn, ou.utxo.Output.Amount)
		if err != nil {
			return nil, errors.New("wallet: input sum overflow during coin selection")
		}
		totalIn = sum
		if totalIn >= needed {
			break
		}
	}
	if totalIn < needed {
		return nil, fmt.Errorf("wallet: insufficient funds: have %d, need %d", totalIn, needed)
	}

	// Build unsigned transaction.
	transaction := &tx.Transaction{Version: 2, ChainID: tx.ChainIDFor(w.IsTestnet)}
	for _, ou := range selected {
		transaction.Inputs = append(transaction.Inputs, tx.TxIn{
			PrevOut:  ou.utxo.OutPoint,
			Sequence: 0xFFFFFFFF,
		})
	}

	// Output to recipient.
	transaction.Outputs = append(transaction.Outputs, tx.TxOut{
		Amount:   amount,
		PkScript: tx.CreateP2PKHLockScript(toPKH),
	})

	// Change output.
	change := totalIn - needed
	if change > 0 {
		transaction.Outputs = append(transaction.Outputs, tx.TxOut{
			Amount:   change,
			PkScript: tx.CreateP2PKHLockScript(w.PubKeyHash()),
		})
	}

	// Sign each input.
	for i, ou := range selected {
		kp := w.Keys[ou.keyIndex]
		pkh := crypto.Hash160(kp.PublicKey.SerializeCompressed())
		subscript := tx.CreateP2PKHLockScript(pkh)
		sigHash := transaction.SigHash(i, subscript)

		sig, err := crypto.Sign(kp.PrivateKey, sigHash)
		if err != nil {
			return nil, fmt.Errorf("wallet: sign input %d: %w", i, err)
		}

		transaction.Inputs[i].SignatureScript = tx.CreateP2PKHUnlockScript(
			sig.Bytes(),
			kp.PublicKey.SerializeCompressed(),
		)
	}

	return transaction, nil
}

// --- internal helpers ---

func generateKeyPair() (KeyPair, error) {
	privKey, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		return KeyPair{}, err
	}
	return KeyPair{
		PrivateKey: privKey,
		PublicKey:  pubKey,
		Address:    crypto.Address(crypto.PubKeyToBech32mAddress(pubKey)),
	}, nil
}
