package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/scrypt"

	"nous/crypto"
)

// WalletState holds the in-memory wallet state.
type WalletState struct {
	mu        sync.Mutex
	filePath  string
	unlocked  bool
	mnemonic  string
	master    *HDKey
	nextIndex uint32
	keys      []DerivedKey
	nodeURL   string
}

// DerivedKey holds metadata about a derived address.
type DerivedKey struct {
	Index   uint32 `json:"index"`
	Path    string `json:"path"`
	Address string `json:"address"`
	Label   string `json:"label,omitempty"`
}

// walletFile is the on-disk format.
type walletFile struct {
	NextIndex uint32       `json:"next_index"`
	Keys      []DerivedKey `json:"keys"`
}

// scrypt params
const (
	scryptN      = 32768
	scryptR      = 8
	scryptP      = 1
	scryptKeyLen = 32
	saltLen      = 32
)

func NewWalletState(walletDir, nodeURL string) *WalletState {
	return &WalletState{
		filePath: filepath.Join(walletDir, "wallet.dat"),
		nodeURL:  nodeURL,
	}
}

func (w *WalletState) WalletExists() bool {
	_, err := os.Stat(w.filePath)
	return err == nil
}

func (w *WalletState) Create(mnemonic, password string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !ValidateMnemonic(mnemonic) {
		return errors.New("invalid mnemonic")
	}

	seed := MnemonicToSeed(mnemonic, "")
	master, err := MasterKeyFromSeed(seed)
	if err != nil {
		return err
	}

	w.mnemonic = mnemonic
	w.master = master
	w.nextIndex = 0
	w.keys = nil
	w.unlocked = true

	// Derive first address
	if err := w.deriveNextLocked(); err != nil {
		return err
	}

	return w.saveLocked(password)
}

func (w *WalletState) Import(mnemonic, password string) error {
	return w.Create(mnemonic, password)
}

func (w *WalletState) Unlock(password string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := os.ReadFile(w.filePath)
	if err != nil {
		return errors.New("wallet file not found")
	}

	// Decrypt: salt(32) + nonce(12) + ciphertext
	if len(data) < saltLen+12+1 {
		return errors.New("corrupt wallet file")
	}
	salt := data[:saltLen]
	nonce := data[saltLen : saltLen+12]
	ciphertext := data[saltLen+12:]

	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return errors.New("wrong password")
	}

	// Parse decrypted data
	var stored struct {
		Mnemonic string     `json:"mnemonic"`
		File     walletFile `json:"file"`
	}
	if err := json.Unmarshal(plaintext, &stored); err != nil {
		return errors.New("corrupt wallet data")
	}

	seed := MnemonicToSeed(stored.Mnemonic, "")
	master, err := MasterKeyFromSeed(seed)
	if err != nil {
		return err
	}

	w.mnemonic = stored.Mnemonic
	w.master = master
	w.nextIndex = stored.File.NextIndex
	w.keys = stored.File.Keys
	w.unlocked = true

	return nil
}

func (w *WalletState) Lock() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.mnemonic = ""
	w.master = nil
	w.unlocked = false
}

func (w *WalletState) IsUnlocked() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.unlocked
}

func (w *WalletState) GetMnemonic() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked {
		return "", errors.New("wallet locked")
	}
	return w.mnemonic, nil
}

func (w *WalletState) DeriveNextAddress(password string) (DerivedKey, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked {
		return DerivedKey{}, errors.New("wallet locked")
	}
	if err := w.deriveNextLocked(); err != nil {
		return DerivedKey{}, err
	}
	if err := w.saveLocked(password); err != nil {
		return DerivedKey{}, err
	}
	return w.keys[len(w.keys)-1], nil
}

func (w *WalletState) deriveNextLocked() error {
	child, err := w.master.DeriveNOUSKey(w.nextIndex)
	if err != nil {
		return err
	}
	dk := DerivedKey{
		Index:   w.nextIndex,
		Path:    formatPath(w.nextIndex),
		Address: child.Address(),
	}
	w.keys = append(w.keys, dk)
	w.nextIndex++
	return nil
}

func formatPath(index uint32) string {
	return "m/44'/999'/0'/0/" + itoa(index)
}

func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func (w *WalletState) ListAddresses() ([]DerivedKey, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked {
		return nil, errors.New("wallet locked")
	}
	out := make([]DerivedKey, len(w.keys))
	copy(out, w.keys)
	return out, nil
}

func (w *WalletState) GetActiveAddress() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked || len(w.keys) == 0 {
		return "", errors.New("wallet locked or empty")
	}
	return w.keys[0].Address, nil
}

// GetKeyForAddress returns the HD key for a specific address.
func (w *WalletState) GetKeyForAddress(addr string) (*HDKey, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked {
		return nil, errors.New("wallet locked")
	}
	for _, dk := range w.keys {
		if dk.Address == addr {
			return w.master.DeriveNOUSKey(dk.Index)
		}
	}
	return nil, errors.New("address not found in wallet")
}

// AllAddresses returns all derived addresses.
func (w *WalletState) AllAddresses() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var addrs []string
	for _, dk := range w.keys {
		addrs = append(addrs, dk.Address)
	}
	return addrs
}

// GetPubKeyHashForAddress returns the 20-byte pubkey hash for signing.
func (w *WalletState) GetPubKeyHashForAddress(addr string) ([]byte, error) {
	key, err := w.GetKeyForAddress(addr)
	if err != nil {
		return nil, err
	}
	return crypto.Hash160(key.PublicKey().SerializeCompressed()), nil
}

func (w *WalletState) saveLocked(password string) error {
	stored := struct {
		Mnemonic string     `json:"mnemonic"`
		File     walletFile `json:"file"`
	}{
		Mnemonic: w.mnemonic,
		File: walletFile{
			NextIndex: w.nextIndex,
			Keys:      w.keys,
		},
	}
	plaintext, _ := json.Marshal(stored)

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}

	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Write salt + nonce + ciphertext
	out := make([]byte, 0, saltLen+len(nonce)+len(ciphertext))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)

	dir := filepath.Dir(w.filePath)
	os.MkdirAll(dir, 0700)

	return os.WriteFile(w.filePath, out, 0600)
}
