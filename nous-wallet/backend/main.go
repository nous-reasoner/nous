package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"nous/crypto"
	"nous/tx"
)

// JSON-RPC types
type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     int             `json:"id"`
}

type Response struct {
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
	ID     int    `json:"id"`
}

var (
	wallet   *WalletState
	password string // kept in memory for save operations after derive
)

func main() {
	homeDir, _ := os.UserHomeDir()
	walletDir := filepath.Join(homeDir, ".nous-wallet")
	nodeURL := "http://localhost:8332/rpc"

	// Check CLI args for node URL
	for i, arg := range os.Args {
		if arg == "-node" && i+1 < len(os.Args) {
			nodeURL = os.Args[i+1]
		}
	}

	wallet = NewWalletState(walletDir, nodeURL)

	log.SetOutput(os.Stderr)
	log.Println("wallet backend started")

	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return
			}
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		resp := handle(req)
		out, _ := json.Marshal(resp)
		fmt.Fprintf(os.Stdout, "%s\n", out)
	}
}

func handle(req Request) Response {
	var params map[string]json.RawMessage
	json.Unmarshal(req.Params, &params)

	getString := func(key string) string {
		raw, ok := params[key]
		if !ok {
			return ""
		}
		var s string
		json.Unmarshal(raw, &s)
		return s
	}
	getInt := func(key string) int64 {
		raw, ok := params[key]
		if !ok {
			return 0
		}
		var n int64
		json.Unmarshal(raw, &n)
		return n
	}

	switch req.Method {

	case "wallet_exists":
		return Response{Result: wallet.WalletExists(), ID: req.ID}

	case "create_wallet":
		wordCount := int(getInt("word_count"))
		if wordCount == 0 {
			wordCount = 12
		}
		pw := getString("password")
		mnemonic, err := GenerateMnemonic(wordCount)
		if err != nil {
			return errResp(req.ID, err)
		}
		if err := wallet.Create(mnemonic, pw); err != nil {
			return errResp(req.ID, err)
		}
		password = pw
		return Response{Result: map[string]string{"mnemonic": mnemonic}, ID: req.ID}

	case "import_wallet":
		mnemonic := getString("mnemonic")
		pw := getString("password")
		if err := wallet.Import(mnemonic, pw); err != nil {
			return errResp(req.ID, err)
		}
		password = pw
		return Response{Result: map[string]string{"status": "ok"}, ID: req.ID}

	case "unlock":
		pw := getString("password")
		if err := wallet.Unlock(pw); err != nil {
			return errResp(req.ID, err)
		}
		password = pw
		return Response{Result: map[string]string{"status": "ok"}, ID: req.ID}

	case "lock":
		wallet.Lock()
		password = ""
		return Response{Result: map[string]string{"status": "ok"}, ID: req.ID}

	case "get_mnemonic":
		m, err := wallet.GetMnemonic()
		if err != nil {
			return errResp(req.ID, err)
		}
		return Response{Result: map[string]string{"mnemonic": m}, ID: req.ID}

	case "derive_address":
		dk, err := wallet.DeriveNextAddress(password)
		if err != nil {
			return errResp(req.ID, err)
		}
		return Response{Result: dk, ID: req.ID}

	case "list_addresses":
		keys, err := wallet.ListAddresses()
		if err != nil {
			return errResp(req.ID, err)
		}
		return Response{Result: keys, ID: req.ID}

	case "get_balance":
		return handleGetBalance(req.ID)

	case "send":
		to := getString("to")
		amount := getInt("amount")
		fee := getInt("fee")
		msg := getString("message")
		return handleSend(req.ID, to, amount, fee, msg)

	case "get_history":
		return handleGetHistory(req.ID)

	case "set_node":
		wallet.nodeURL = getString("url")
		return Response{Result: map[string]string{"status": "ok"}, ID: req.ID}

	default:
		return Response{Error: "unknown method: " + req.Method, ID: req.ID}
	}
}

func errResp(id int, err error) Response {
	return Response{Error: err.Error(), ID: id}
}

// --- Node RPC ---

func nodeRPC(method string, params ...any) (json.RawMessage, error) {
	body, _ := json.Marshal(struct {
		Method string `json:"method"`
		Params []any  `json:"params"`
		ID     int    `json:"id"`
	}{method, params, 1})

	resp, err := http.Post(wallet.nodeURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("node unreachable: %v", err)
	}
	defer resp.Body.Close()

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("%s", rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

// --- Balance ---

func handleGetBalance(id int) Response {
	addrs := wallet.AllAddresses()
	if len(addrs) == 0 {
		return errResp(id, fmt.Errorf("wallet locked or empty"))
	}

	var totalBalance, totalImmature int64
	for _, addr := range addrs {
		raw, err := nodeRPC("getbalance", addr)
		if err != nil {
			continue
		}
		var bal struct {
			Balance  int64 `json:"balance"`
			Immature int64 `json:"immature"`
		}
		json.Unmarshal(raw, &bal)
		totalBalance += bal.Balance
		totalImmature += bal.Immature
	}

	return Response{Result: map[string]int64{
		"balance":  totalBalance,
		"immature": totalImmature,
	}, ID: id}
}

// --- Send Transaction ---

func handleSend(id int, toAddr string, amount, fee int64, message string) Response {
	if !wallet.IsUnlocked() {
		return errResp(id, fmt.Errorf("wallet locked"))
	}

	// Validate recipient address
	_, toPKH, err := crypto.Bech32mAddressToPubKeyHash(toAddr)
	if err != nil {
		return errResp(id, fmt.Errorf("invalid address: %v", err))
	}
	_ = toPKH

	// Collect UTXOs from all addresses
	type utxoInfo struct {
		TxID      string `json:"txid"`
		Index     uint32 `json:"index"`
		Value     int64  `json:"value"`
		Height    uint64 `json:"height"`
		Address   string // which wallet address owns this
	}

	var allUTXOs []utxoInfo
	addrs := wallet.AllAddresses()
	for _, addr := range addrs {
		raw, err := nodeRPC("listunspent", addr)
		if err != nil {
			continue
		}
		var utxos []struct {
			TxID   string `json:"txid"`
			Index  uint32 `json:"index"`
			Value  int64  `json:"value"`
			Height uint64 `json:"height"`
		}
		json.Unmarshal(raw, &utxos)
		for _, u := range utxos {
			allUTXOs = append(allUTXOs, utxoInfo{
				TxID: u.TxID, Index: u.Index, Value: u.Value,
				Height: u.Height, Address: addr,
			})
		}
	}

	// Coin selection (largest first)
	needed := amount + fee
	var selected []utxoInfo
	var inputSum int64

	// Sort by value descending
	for i := 0; i < len(allUTXOs); i++ {
		for j := i + 1; j < len(allUTXOs); j++ {
			if allUTXOs[j].Value > allUTXOs[i].Value {
				allUTXOs[i], allUTXOs[j] = allUTXOs[j], allUTXOs[i]
			}
		}
	}

	for _, u := range allUTXOs {
		selected = append(selected, u)
		inputSum += u.Value
		if inputSum >= needed {
			break
		}
	}

	if inputSum < needed {
		return errResp(id, fmt.Errorf("insufficient balance: have %d, need %d", inputSum, needed))
	}

	// Build transaction
	_, toPKHBytes, _ := crypto.Bech32mAddressToPubKeyHash(toAddr)
	toPKH = toPKHBytes


	var inputs []tx.TxIn
	for _, u := range selected {
		txidBytes, _ := hex.DecodeString(u.TxID)
		var txid crypto.Hash
		copy(txid[:], txidBytes)
		inputs = append(inputs, tx.TxIn{
			PrevOut: tx.OutPoint{TxID: txid, Index: u.Index},
		})
	}

	// Outputs
	var outputs []tx.TxOut

	// Recipient output
	outputs = append(outputs, tx.TxOut{
		Amount:   amount,
		PkScript: tx.CreateP2PKHLockScript(toPKH),
	})

	// OP_RETURN message (if present)
	if message != "" {
		outputs = append(outputs, tx.TxOut{
			Amount:   0,
			PkScript: tx.CreateOpReturnScript([]byte(message)),
		})
	}

	// Change output
	change := inputSum - amount - fee
	if change > 546 { // dust limit
		changePKH, _ := wallet.GetPubKeyHashForAddress(addrs[0])
		outputs = append(outputs, tx.TxOut{
			Amount:   change,
			PkScript: tx.CreateP2PKHLockScript(changePKH),
		})
	}

	transaction := &tx.Transaction{
		Version:  1,
		ChainID:  tx.ChainIDMainnet,
		Inputs:   inputs,
		Outputs:  outputs,
	}

	// Sign each input
	for i, sel := range selected {
		hdKey, err := wallet.GetKeyForAddress(sel.Address)
		if err != nil {
			return errResp(id, fmt.Errorf("sign: %v", err))
		}
		privKey := hdKey.PrivateKey()
		pubKey := hdKey.PublicKey()

		sigHash := transaction.SigHash(i, tx.CreateP2PKHLockScript(
			crypto.Hash160(pubKey.SerializeCompressed()),
		))
		sig, err := crypto.Sign(privKey, sigHash)
		if err != nil {
			return errResp(id, fmt.Errorf("sign: %v", err))
		}

		transaction.Inputs[i].SignatureScript = tx.CreateP2PKHUnlockScript(
			sig.Bytes(), pubKey.SerializeCompressed(),
		)
	}

	// Serialize and broadcast
	rawTx := hex.EncodeToString(transaction.Serialize())
	result, err := nodeRPC("sendrawtx", rawTx)
	if err != nil {
		return errResp(id, fmt.Errorf("broadcast: %v", err))
	}

	var txid string
	json.Unmarshal(result, &txid)

	return Response{Result: map[string]string{
		"txid":   txid,
		"status": "sent",
	}, ID: id}
}

// --- Transaction History ---

func handleGetHistory(id int) Response {
	addrs := wallet.AllAddresses()
	if len(addrs) == 0 {
		return errResp(id, fmt.Errorf("wallet locked or empty"))
	}

	// Get current height
	raw, err := nodeRPC("getblockcount")
	if err != nil {
		return errResp(id, err)
	}
	var height int
	json.Unmarshal(raw, &height)

	// Scan last 200 blocks for transactions involving our addresses
	addrSet := map[string]bool{}
	for _, a := range addrs {
		addrSet[a] = true
	}

	type historyEntry struct {
		TxID      string `json:"txid"`
		Height    int    `json:"height"`
		Timestamp int64  `json:"timestamp"`
		Amount    int64  `json:"amount"` // positive = received, negative = sent
		Address   string `json:"address"`
		Message   string `json:"message,omitempty"`
	}

	var history []historyEntry
	scanFrom := height - 200
	if scanFrom < 0 {
		scanFrom = 0
	}

	for h := height; h >= scanFrom && len(history) < 50; h-- {
		blockRaw, err := nodeRPC("getblock", h)
		if err != nil {
			continue
		}
		var block struct {
			Height       int      `json:"height"`
			Timestamp    int64    `json:"timestamp"`
			Transactions []string `json:"transactions"`
			MinerAddress string   `json:"miner_address"`
		}
		json.Unmarshal(blockRaw, &block)

		// Check if coinbase went to our address
		if addrSet[block.MinerAddress] {
			history = append(history, historyEntry{
				TxID:      block.Transactions[0],
				Height:    block.Height,
				Timestamp: block.Timestamp,
				Amount:    100_000_000, // 1 NOUS
				Address:   block.MinerAddress,
				Message:   "Block reward",
			})
		}

		// Check other transactions
		for _, txid := range block.Transactions[1:] {
			txRaw, err := nodeRPC("gettx", txid)
			if err != nil {
				continue
			}
			var t struct {
				Outputs []struct {
					Value  int64  `json:"value"`
					Script string `json:"script"`
				} `json:"outputs"`
			}
			json.Unmarshal(txRaw, &t)

			for _, out := range t.Outputs {
				// Check OP_RETURN for message
				if len(out.Script) > 4 && out.Script[:2] == "6a" {
					// OP_RETURN detected
					continue
				}
				// Check P2PKH
				pkh := scriptToPKH(out.Script)
				if pkh == "" {
					continue
				}
				addr := crypto.PubKeyHashToBech32mAddress(hexToBytes(pkh))
				if addrSet[addr] {
					history = append(history, historyEntry{
						TxID:      txid,
						Height:    block.Height,
						Timestamp: block.Timestamp,
						Amount:    out.Value,
						Address:   addr,
					})
				}
			}
		}
	}

	return Response{Result: history, ID: id}
}

func scriptToPKH(script string) string {
	if len(script) == 50 && strings.HasPrefix(script, "76a914") && strings.HasSuffix(script, "88ac") {
		return script[6:46]
	}
	return ""
}

func hexToBytes(s string) []byte {
	b, _ := hex.DecodeString(s)
	return b
}
