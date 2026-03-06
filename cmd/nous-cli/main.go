// nous-cli is the command-line client for interacting with a NOUS node.
//
// Usage:
//
//	nous-cli [global flags] <command> [args]
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"nous/crypto"
	"nous/tx"
	"nous/wallet"
)

const version = "0.1.0-dev"

// NOU is the base unit: 1 NOUS = 1e8 nou.
const NOU = int64(1_0000_0000)

// DefaultFee is the default transaction fee (0.001 NOUS).
const DefaultFee = int64(100_0000)

// global flags
var (
	flagRPCHost    = "localhost"
	flagRPCPort    = 9332
	flagWalletFile = ""
	flagWalletPass = ""
	flagJSON       = false
	flagTestnet    = false
)

func main() {
	// Parse global flags manually from os.Args.
	args := os.Args[1:]
	args = parseGlobalFlags(args)

	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	cmd := args[0]
	cmdArgs := args[1:]

	var err error
	switch cmd {
	case "version":
		fmt.Printf("nous-cli %s\n", version)
		return
	case "createwallet":
		err = cmdCreateWallet()
	case "getaddress":
		err = cmdGetAddress()
	case "newaddress":
		err = cmdNewAddress()
	case "getbalance":
		err = cmdGetBalance(cmdArgs)
	case "send":
		err = cmdSend(cmdArgs)
	case "getblockcount":
		err = cmdGetBlockCount()
	case "getblock":
		err = cmdGetBlock(cmdArgs)
	case "getmininginfo":
		err = cmdGetMiningInfo()
	case "getpeerinfo":
		err = cmdGetPeerInfo()
	case "exportprivkey":
		err = cmdExportPrivKey()
	case "importprivkey":
		err = cmdImportPrivKey(cmdArgs)
	case "backupwallet":
		err = cmdBackupWallet(cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func parseGlobalFlags(args []string) []string {
	var rest []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--rpchost":
			i++
			if i < len(args) {
				flagRPCHost = args[i]
			}
		case "--rpcport":
			i++
			if i < len(args) {
				v, err := strconv.Atoi(args[i])
				if err == nil {
					flagRPCPort = v
				}
			}
		case "--walletfile":
			i++
			if i < len(args) {
				flagWalletFile = args[i]
			}
		case "--walletpass":
			i++
			if i < len(args) {
				flagWalletPass = args[i]
			}
		case "--json":
			flagJSON = true
		case "--testnet":
			flagTestnet = true
		default:
			rest = append(rest, args[i])
		}
	}
	return rest
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `nous-cli %s — command-line client for NOUS

Usage: nous-cli [flags] <command> [args]

Global flags:
  --rpchost <host>       RPC server host (default: localhost)
  --rpcport <port>       RPC server port (default: 9332)
  --walletfile <path>    wallet file path (default: ~/.nous/wallet.dat)
  --walletpass <pass>    wallet password
  --testnet              use testnet chain ID
  --json                 output in JSON format

Commands:
  createwallet           create a new wallet
  getaddress             show primary address
  newaddress             generate a new address
  getbalance [address]   get balance (wallet or address)
  send <address> <amount> send NOUS to address
  exportprivkey          export primary private key (hex)
  importprivkey <hex>    import private key and save wallet
  backupwallet <path>    copy wallet file to destination
  getblockcount          get current block height
  getblock <height>      get block by height
  getmininginfo          get mining information
  getpeerinfo            get connected peers
  version                show version
`, version)
}

func defaultWalletPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "wallet.dat"
	}
	return filepath.Join(home, ".nous", "wallet.dat")
}

func walletPath() string {
	if flagWalletFile != "" {
		return flagWalletFile
	}
	return defaultWalletPath()
}

func rpcClient() *RPCClient {
	return NewRPCClient(flagRPCHost, flagRPCPort)
}

// --- wallet commands ---

func cmdCreateWallet() error {
	path := walletPath()

	// Ensure parent directory exists.
	if dir := filepath.Dir(path); dir != "" {
		os.MkdirAll(dir, 0700)
	}

	w, err := wallet.NewWallet()
	if err != nil {
		return fmt.Errorf("create wallet: %w", err)
	}

	pass := flagWalletPass
	if pass == "" {
		pass = "default"
	}

	if err := w.SaveToFile(path, pass); err != nil {
		return fmt.Errorf("save wallet: %w", err)
	}

	addr := w.GetAddress()
	if flagJSON {
		return printJSON(map[string]interface{}{
			"file":    path,
			"address": string(addr),
		})
	}
	fmt.Printf("wallet created: %s\n", path)
	fmt.Printf("address: %s\n", addr)
	return nil
}

func cmdGetAddress() error {
	w, err := loadWallet()
	if err != nil {
		return err
	}
	addr := w.GetAddress()
	if flagJSON {
		return printJSON(map[string]string{"address": string(addr)})
	}
	fmt.Println(addr)
	return nil
}

func cmdNewAddress() error {
	w, err := loadWallet()
	if err != nil {
		return err
	}
	idx, err := w.GenerateNewKey()
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	newAddr := w.Keys[idx].Address

	pass := flagWalletPass
	if pass == "" {
		pass = "default"
	}
	if err := w.SaveToFile(walletPath(), pass); err != nil {
		return fmt.Errorf("save wallet: %w", err)
	}

	if flagJSON {
		return printJSON(map[string]interface{}{
			"index":   idx,
			"address": string(newAddr),
		})
	}
	fmt.Printf("new address [%d]: %s\n", idx, newAddr)
	return nil
}

// --- balance & send ---

func cmdGetBalance(args []string) error {
	client := rpcClient()

	// If address provided, use it; otherwise use wallet's primary address.
	var addr string
	if len(args) > 0 {
		addr = args[0]
	} else {
		w, err := loadWallet()
		if err != nil {
			return err
		}
		addr = string(w.GetAddress())
	}

	var result struct {
		Balance  int64 `json:"balance"`
		Immature int64 `json:"immature"`
	}
	if err := client.CallInto(&result, "getbalance", []string{addr}); err != nil {
		return fmt.Errorf("getbalance: %w", err)
	}

	if flagJSON {
		return printJSON(map[string]interface{}{
			"address":  addr,
			"balance":  result.Balance,
			"immature": result.Immature,
			"nous":     formatNOUS(result.Balance),
		})
	}
	fmt.Printf("%s NOUS\n", formatNOUS(result.Balance))
	if result.Immature > 0 {
		fmt.Printf("(%s NOUS immature)\n", formatNOUS(result.Immature))
	}
	return nil
}

func cmdSend(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: send <address> <amount>")
	}
	toAddr := crypto.Address(args[0])
	amount, err := parseNOUS(args[1])
	if err != nil {
		return fmt.Errorf("invalid amount: %w", err)
	}

	w, err := loadWallet()
	if err != nil {
		return err
	}

	client := rpcClient()
	addr := string(w.GetAddress())

	// Fetch UTXOs from node.
	var utxoList []struct {
		TxID   string `json:"txid"`
		Index  uint32 `json:"index"`
		Value  int64  `json:"value"`
		Script string `json:"script"`
		Height uint64 `json:"height"`
	}
	if err := client.CallInto(&utxoList, "listunspent", []string{addr}); err != nil {
		return fmt.Errorf("listunspent: %w", err)
	}

	// Build a local UTXOSet from the RPC results.
	utxoSet := tx.NewUTXOSet()
	for _, u := range utxoList {
		txIDBytes, err := hex.DecodeString(u.TxID)
		if err != nil {
			continue
		}
		var txID crypto.Hash
		copy(txID[:], txIDBytes)
		script, err := hex.DecodeString(u.Script)
		if err != nil {
			continue
		}
		utxoSet.Add(
			tx.OutPoint{TxID: txID, Index: u.Index},
			tx.TxOut{Amount: u.Value, PkScript: script},
			u.Height,
			false, // coinbase flag not tracked in RPC; maturity enforced at block validation
		)
	}

	// Build and sign the transaction.
	transaction, err := w.CreateTransaction(toAddr, amount, DefaultFee, utxoSet)
	if err != nil {
		return fmt.Errorf("create tx: %w", err)
	}

	// Serialize and send.
	rawHex := hex.EncodeToString(transaction.Serialize())
	var txID string
	if err := client.CallInto(&txID, "sendrawtx", []string{rawHex}); err != nil {
		return fmt.Errorf("sendrawtx: %w", err)
	}

	if flagJSON {
		return printJSON(map[string]interface{}{
			"txid":   txID,
			"amount": amount,
			"fee":    DefaultFee,
			"to":     string(toAddr),
		})
	}
	fmt.Printf("txid: %s\n", txID)
	return nil
}

// --- node query commands ---

func cmdGetBlockCount() error {
	client := rpcClient()
	var height uint64
	if err := client.CallInto(&height, "getblockcount", nil); err != nil {
		return fmt.Errorf("getblockcount: %w", err)
	}
	if flagJSON {
		return printJSON(map[string]interface{}{"height": height})
	}
	fmt.Println(height)
	return nil
}

func cmdGetBlock(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: getblock <height>")
	}
	height, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid height: %w", err)
	}

	client := rpcClient()
	var blk map[string]interface{}
	if err := client.CallInto(&blk, "getblock", []uint64{height}); err != nil {
		return fmt.Errorf("getblock: %w", err)
	}

	if flagJSON {
		return printJSON(blk)
	}

	fmt.Printf("hash:        %s\n", blk["hash"])
	fmt.Printf("height:      %v\n", blk["height"])
	fmt.Printf("version:     %v\n", blk["version"])
	fmt.Printf("timestamp:   %v\n", blk["timestamp"])
	fmt.Printf("prev_hash:   %s\n", blk["prev_hash"])
	fmt.Printf("merkle_root: %s\n", blk["merkle_root"])
	fmt.Printf("difficulty:  %v\n", blk["difficulty"])
	fmt.Printf("nonce:       %v\n", blk["nonce"])
	fmt.Printf("tx_count:    %v\n", blk["tx_count"])
	if txs, ok := blk["transactions"].([]interface{}); ok {
		for i, t := range txs {
			fmt.Printf("  tx[%d]: %s\n", i, t)
		}
	}
	return nil
}

func cmdGetMiningInfo() error {
	client := rpcClient()
	var info map[string]interface{}
	if err := client.CallInto(&info, "getmininginfo", nil); err != nil {
		return fmt.Errorf("getmininginfo: %w", err)
	}
	if flagJSON {
		return printJSON(info)
	}
	fmt.Printf("height:          %v\n", info["height"])
	fmt.Printf("difficulty_bits: %v\n", info["difficulty_bits"])
	fmt.Printf("vdf_iterations:  %v\n", info["vdf_iterations"])
	fmt.Printf("mempool_size:    %v\n", info["mempool_size"])
	fmt.Printf("mining:          %v\n", info["mining"])
	return nil
}

func cmdGetPeerInfo() error {
	client := rpcClient()
	var peers []map[string]interface{}
	if err := client.CallInto(&peers, "getpeerinfo", nil); err != nil {
		return fmt.Errorf("getpeerinfo: %w", err)
	}
	if flagJSON {
		return printJSON(peers)
	}
	if len(peers) == 0 {
		fmt.Println("no peers connected")
		return nil
	}
	for i, p := range peers {
		fmt.Printf("peer %d:\n", i)
		fmt.Printf("  addr:         %v\n", p["addr"])
		fmt.Printf("  inbound:      %v\n", p["inbound"])
		fmt.Printf("  version:      %v\n", p["version"])
		fmt.Printf("  block_height: %v\n", p["block_height"])
		fmt.Printf("  handshaked:   %v\n", p["handshaked"])
	}
	return nil
}

// --- wallet export/import ---

func cmdExportPrivKey() error {
	w, err := loadWallet()
	if err != nil {
		return err
	}
	privHex := hex.EncodeToString(w.ExportPrivateKey())
	addr := w.GetAddress()

	if flagJSON {
		return printJSON(map[string]interface{}{
			"private_key": privHex,
			"address":     string(addr),
		})
	}
	fmt.Printf("Private Key (hex): %s\n", privHex)
	fmt.Printf("Address: %s\n", addr)
	fmt.Println("WARNING: Never share your private key. Anyone with this key can spend your NOUS.")
	return nil
}

func cmdImportPrivKey(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: importprivkey <hex>")
	}
	privBytes, err := hex.DecodeString(args[0])
	if err != nil || len(privBytes) != 32 {
		return fmt.Errorf("invalid private key: must be 32 bytes hex (64 characters)")
	}

	path := walletPath()

	// Ensure parent directory exists.
	if dir := filepath.Dir(path); dir != "" {
		os.MkdirAll(dir, 0700)
	}

	// Create a fresh wallet and import the key as primary.
	w := &wallet.Wallet{IsTestnet: flagTestnet}
	idx, err := w.ImportPrivateKey(privBytes)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	w.Primary = idx

	pass := flagWalletPass
	if pass == "" {
		pass = "default"
	}
	if err := w.SaveToFile(path, pass); err != nil {
		return fmt.Errorf("save wallet: %w", err)
	}

	addr := w.GetAddress()
	if flagJSON {
		return printJSON(map[string]interface{}{
			"file":    path,
			"address": string(addr),
		})
	}
	fmt.Printf("imported key to: %s\n", path)
	fmt.Printf("address: %s\n", addr)
	return nil
}

func cmdBackupWallet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: backupwallet <destination_path>")
	}
	src := walletPath()
	dst := args[0]

	// Ensure destination directory exists.
	if dir := filepath.Dir(dst); dir != "" {
		os.MkdirAll(dir, 0700)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open wallet: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	if flagJSON {
		return printJSON(map[string]interface{}{
			"source":      src,
			"destination": dst,
		})
	}
	fmt.Printf("wallet backed up: %s → %s\n", src, dst)
	return nil
}

// --- helpers ---

func loadWallet() (*wallet.Wallet, error) {
	pass := flagWalletPass
	if pass == "" {
		pass = "default"
	}
	w, err := wallet.LoadFromFile(walletPath(), pass)
	if err != nil {
		return nil, fmt.Errorf("load wallet: %w", err)
	}
	w.IsTestnet = flagTestnet
	return w, nil
}

func formatNOUS(nou int64) string {
	whole := nou / NOU
	frac := nou % NOU
	if frac < 0 {
		frac = -frac
	}
	return fmt.Sprintf("%d.%08d", whole, frac)
}

func parseNOUS(s string) (int64, error) {
	var whole, frac int64
	parts := splitDot(s)
	w, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	whole = w * NOU

	if len(parts) > 1 {
		fracStr := parts[1]
		for len(fracStr) < 8 {
			fracStr += "0"
		}
		fracStr = fracStr[:8]
		f, err := strconv.ParseInt(fracStr, 10, 64)
		if err != nil {
			return 0, err
		}
		frac = f
	}
	return whole + frac, nil
}

func splitDot(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func printJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
