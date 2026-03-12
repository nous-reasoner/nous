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
	"sync"
	"time"

	"nous/crypto"
	"nous/tx"
)

var stdoutMu sync.Mutex

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
	nodeURL := "http://rpc.nouschain.org/api"

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
		go func(r Request) {
			resp := handle(r)
			stdoutMu.Lock()
			out, _ := json.Marshal(resp)
			fmt.Fprintf(os.Stdout, "%s\n", out)
			stdoutMu.Unlock()
		}(req)
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
		from := getString("from")
		to := getString("to")
		amount := getInt("amount")
		fee := getInt("fee")
		msg := getString("message")
		return handleSend(req.ID, from, to, amount, fee, msg)

	case "get_history":
		return handleGetHistory(req.ID)

	case "get_private_key":
		addr := getString("address")
		hdKey, err := wallet.GetKeyForAddress(addr)
		if err != nil {
			return errResp(req.ID, err)
		}
		privKeyHex := hex.EncodeToString(hdKey.PrivateKey().Bytes())
		return Response{Result: map[string]string{"private_key": privKeyHex}, ID: req.ID}

	case "import_private_key":
		privKey := getString("private_key")
		dk, err := wallet.ImportPrivateKey(privKey, password)
		if err != nil {
			return errResp(req.ID, err)
		}
		return Response{Result: dk, ID: req.ID}

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

func handleSend(id int, fromAddr, toAddr string, amount, fee int64, message string) Response {
	if !wallet.IsUnlocked() {
		return errResp(id, fmt.Errorf("wallet locked"))
	}

	// Validate recipient address
	_, toPKH, err := crypto.Bech32mAddressToPubKeyHash(toAddr)
	if err != nil {
		return errResp(id, fmt.Errorf("invalid address: %v", err))
	}

	// Use active address as sender if not specified
	if fromAddr == "" {
		addr, err := wallet.GetActiveAddress()
		if err != nil {
			return errResp(id, err)
		}
		fromAddr = addr
	}

	// Collect UTXOs only from the sender address
	type utxoInfo struct {
		TxID      string `json:"txid"`
		Index     uint32 `json:"index"`
		Value     int64  `json:"value"`
		Height    uint64 `json:"height"`
		Address   string
	}

	var allUTXOs []utxoInfo
	raw, err := nodeRPC("listunspent", fromAddr)
	if err != nil {
		return errResp(id, fmt.Errorf("cannot fetch UTXOs: %v", err))
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
			Height: u.Height, Address: fromAddr,
		})
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

	// Change output — send back to sender
	change := inputSum - amount - fee
	if change > 546 { // dust limit
		changePKH, _ := wallet.GetPubKeyHashForAddress(fromAddr)
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

	// Store transaction record locally
	wallet.AddTxRecord(TxRecord{
		TxID:      txid,
		To:        toAddr,
		From:      selected[0].Address,
		Amount:    -amount,
		Fee:       fee,
		Message:   message,
		Timestamp: time.Now().Unix(),
	}, password)

	return Response{Result: map[string]string{
		"txid":   txid,
		"status": "sent",
	}, ID: id}
}

// --- Transaction History ---

func handleGetHistory(id int) Response {
	records, err := wallet.GetTxHistory()
	if err != nil {
		return errResp(id, err)
	}
	return Response{Result: records, ID: id}
}

