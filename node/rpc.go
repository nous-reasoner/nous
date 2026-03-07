package node

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"nous/block"
	"nous/consensus"
	"nous/crypto"
	"nous/network"
	"nous/sat"
	"nous/storage"
	"nous/tx"
	"nous/wallet"
)

// JSON-RPC 2.0 types.

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// RPCServer serves JSON-RPC over HTTP.
type RPCServer struct {
	chain    *consensus.ChainState
	server   *network.Server
	store    *storage.BlockStore
	reasoner *Reasoner
	wallet   *wallet.Wallet
	http     *http.Server
	addr     string // actual bound address after Start
}

// NewRPCServer creates a new RPC server.
func NewRPCServer(
	listenAddr string,
	chain *consensus.ChainState,
	server *network.Server,
	store *storage.BlockStore,
	reasoner *Reasoner,
) *RPCServer {
	rpc := &RPCServer{
		chain:    chain,
		server:   server,
		store:    store,
		reasoner: reasoner,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", rpc.handleRPC)
	rpc.http = &http.Server{Addr: listenAddr, Handler: mux}
	return rpc
}

// Start begins serving RPC requests.
func (r *RPCServer) Start() error {
	ln, err := net.Listen("tcp", r.http.Addr)
	if err != nil {
		return fmt.Errorf("rpc: listen: %w", err)
	}
	r.addr = ln.Addr().String()
	log.Printf("rpc: listening on %s", r.addr)
	go func() {
		if err := r.http.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("rpc: %v", err)
		}
	}()
	return nil
}

// Addr returns the actual bound address (useful when started with port 0).
func (r *RPCServer) Addr() string {
	return r.addr
}

// Stop gracefully shuts down the RPC server.
func (r *RPCServer) Stop() error {
	return r.http.Shutdown(context.Background())
}

// SetWallet sets the wallet for transaction signing (sendtoaddress).
func (r *RPCServer) SetWallet(w *wallet.Wallet) {
	r.wallet = w
}

func (r *RPCServer) handleRPC(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var rpcReq rpcRequest
	if err := json.NewDecoder(req.Body).Decode(&rpcReq); err != nil {
		writeJSON(w, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}, ID: nil})
		return
	}

	var result interface{}
	var rpcErr *rpcError

	switch rpcReq.Method {
	case "getblockcount":
		result = r.chain.Height
	case "getblock":
		result, rpcErr = r.handleGetBlock(rpcReq.Params)
	case "getblockhash":
		result, rpcErr = r.handleGetBlockHash(rpcReq.Params)
	case "gettx":
		result, rpcErr = r.handleGetTx(rpcReq.Params)
	case "sendrawtx":
		result, rpcErr = r.handleSendRawTx(rpcReq.Params)
	case "getbalance":
		result, rpcErr = r.handleGetBalance(rpcReq.Params)
	case "getmininginfo":
		result = r.handleGetMiningInfo()
	case "getpeerinfo":
		result = r.handleGetPeerInfo()
	case "listunspent":
		result, rpcErr = r.handleListUnspent(rpcReq.Params)
	case "sendtoaddress":
		result, rpcErr = r.handleSendToAddress(rpcReq.Params)
	case "getaddress":
		result, rpcErr = r.handleGetAddress()
	case "gettotalsupply":
		result = r.handleGetTotalSupply()
	case "getwork":
		result, rpcErr = r.handleGetWork(rpcReq.Params)
	case "submitwork":
		result, rpcErr = r.handleSubmitWork(rpcReq.Params)
	default:
		rpcErr = &rpcError{Code: -32601, Message: "method not found"}
	}

	resp := rpcResponse{JSONRPC: "2.0", ID: rpcReq.ID}
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		resp.Result = result
	}
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// --- individual method handlers ---

func (r *RPCServer) handleGetBlock(params json.RawMessage) (interface{}, *rpcError) {
	var args []uint64
	if err := json.Unmarshal(params, &args); err != nil || len(args) < 1 {
		return nil, &rpcError{Code: -32602, Message: "params: [height]"}
	}
	height := args[0]
	blk, err := r.store.LoadBlockByHeight(height)
	if err != nil {
		return nil, &rpcError{Code: -32000, Message: err.Error()}
	}
	hash := blk.Header.Hash()
	txIDs := make([]string, len(blk.Transactions))
	for i, t := range blk.Transactions {
		txIDs[i] = hex.EncodeToString(t.TxID().Bytes())
	}
	// Extract miner address from coinbase tx output.
	minerAddr := ""
	if len(blk.Transactions) > 0 {
		cb := blk.Transactions[0]
		if len(cb.Outputs) > 0 {
			if pkh := tx.ExtractPubKeyHashFromP2PKH(cb.Outputs[0].PkScript); pkh != nil {
				minerAddr = crypto.PubKeyHashToBech32mAddress(pkh)
			}
		}
	}
	return map[string]interface{}{
		"hash":          hex.EncodeToString(hash[:]),
		"height":        height,
		"version":       blk.Header.Version,
		"timestamp":     blk.Header.Timestamp,
		"prev_hash":     hex.EncodeToString(blk.Header.PrevBlockHash[:]),
		"merkle_root":   hex.EncodeToString(blk.Header.MerkleRoot[:]),
		"difficulty":    blk.Header.DifficultyBits,
		"seed":          blk.Header.Seed,
		"tx_count":      len(blk.Transactions),
		"transactions":  txIDs,
		"miner_address": minerAddr,
	}, nil
}

func (r *RPCServer) handleGetBlockHash(params json.RawMessage) (interface{}, *rpcError) {
	var args []uint64
	if err := json.Unmarshal(params, &args); err != nil || len(args) < 1 {
		return nil, &rpcError{Code: -32602, Message: "params: [height]"}
	}
	blk, err := r.store.LoadBlockByHeight(args[0])
	if err != nil {
		return nil, &rpcError{Code: -32000, Message: err.Error()}
	}
	hash := blk.Header.Hash()
	return hex.EncodeToString(hash[:]), nil
}

func (r *RPCServer) handleGetTx(params json.RawMessage) (interface{}, *rpcError) {
	var args []string
	if err := json.Unmarshal(params, &args); err != nil || len(args) < 1 {
		return nil, &rpcError{Code: -32602, Message: "params: [txid_hex]"}
	}
	hashBytes, err := hex.DecodeString(args[0])
	if err != nil || len(hashBytes) != 32 {
		return nil, &rpcError{Code: -32602, Message: "invalid txid"}
	}
	var txID crypto.Hash
	copy(txID[:], hashBytes)

	// Check mempool first.
	if t := r.server.Mempool().Get(txID); t != nil {
		return txToJSON(t, -1), nil
	}

	// Search stored blocks (linear scan — acceptable for now).
	for h := uint64(0); h <= r.chain.Height; h++ {
		blk, err := r.store.LoadBlockByHeight(h)
		if err != nil {
			continue
		}
		for _, t := range blk.Transactions {
			if t.TxID() == txID {
				return txToJSON(t, int64(h)), nil
			}
		}
	}
	return nil, &rpcError{Code: -32000, Message: "transaction not found"}
}

func (r *RPCServer) handleSendRawTx(params json.RawMessage) (interface{}, *rpcError) {
	var args []string
	if err := json.Unmarshal(params, &args); err != nil || len(args) < 1 {
		return nil, &rpcError{Code: -32602, Message: "params: [raw_tx_hex]"}
	}
	raw, err := hex.DecodeString(args[0])
	if err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid hex"}
	}
	transaction, err := tx.Deserialize(raw)
	if err != nil {
		return nil, &rpcError{Code: -32000, Message: fmt.Sprintf("decode: %v", err)}
	}
	if err := tx.ValidateTransaction(transaction, r.chain.UTXOSet, r.chain.Height+1, r.chain.IsTestnet); err != nil {
		return nil, &rpcError{Code: -32000, Message: fmt.Sprintf("validate: %v", err)}
	}
	r.server.Mempool().Add(transaction)
	txID := transaction.TxID()

	// Broadcast to peers.
	r.server.BroadcastMessage(&network.MsgTx{Payload: raw})
	log.Printf("rpc: sendrawtx %x broadcast to peers", txID[:8])

	return hex.EncodeToString(txID[:]), nil
}

func (r *RPCServer) handleGetBalance(params json.RawMessage) (interface{}, *rpcError) {
	var args []string
	if err := json.Unmarshal(params, &args); err != nil || len(args) < 1 {
		return nil, &rpcError{Code: -32602, Message: "params: [address]"}
	}
	pkh, err := crypto.DecodePubKeyHash(args[0])
	if err != nil {
		return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("invalid address: %v", err)}
	}
	utxos := r.chain.UTXOSet.FindByPubKeyHash(pkh)
	currentHeight := r.chain.Height
	var balance, immature int64
	for _, u := range utxos {
		if u.IsCoinbase && currentHeight < u.Height+tx.CoinbaseMaturityFor(r.chain.IsTestnet) {
			immature += u.Output.Amount
		} else {
			balance += u.Output.Amount
		}
	}
	return map[string]interface{}{
		"balance":  balance,
		"immature": immature,
	}, nil
}

func (r *RPCServer) handleGetMiningInfo() interface{} {
	diff := r.chain.GetDifficulty()
	return map[string]interface{}{
		"height":          r.chain.Height,
		"difficulty_bits": consensus.TargetToCompact(diff.PoWTarget),
		"mempool_size":    r.server.Mempool().Count(),
		"reasoning":       r.reasoner != nil && r.reasoner.IsRunning(),
	}
}

func (r *RPCServer) handleGetPeerInfo() interface{} {
	peers := r.server.Peers().All()
	info := make([]map[string]interface{}, 0, len(peers))
	for _, p := range peers {
		info = append(info, map[string]interface{}{
			"addr":         p.Addr,
			"inbound":      p.Inbound,
			"version":      p.Version,
			"block_height": p.BlockHeight,
			"handshaked":   p.Handshaked,
		})
	}
	return info
}

func (r *RPCServer) handleListUnspent(params json.RawMessage) (interface{}, *rpcError) {
	var args []string
	if err := json.Unmarshal(params, &args); err != nil || len(args) < 1 {
		return nil, &rpcError{Code: -32602, Message: "params: [address]"}
	}
	pkh, err := crypto.DecodePubKeyHash(args[0])
	if err != nil {
		return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("invalid address: %v", err)}
	}
	utxos := r.chain.UTXOSet.FindByPubKeyHash(pkh)
	currentHeight := r.chain.Height
	result := make([]map[string]interface{}, 0, len(utxos))
	for _, u := range utxos {
		// Skip immature coinbase outputs so callers never build
		// transactions that the node would reject.
		if u.IsCoinbase && currentHeight < u.Height+tx.CoinbaseMaturityFor(r.chain.IsTestnet) {
			continue
		}
		result = append(result, map[string]interface{}{
			"txid":        hex.EncodeToString(u.OutPoint.TxID[:]),
			"index":       u.OutPoint.Index,
			"value":       u.Output.Amount,
			"script":      hex.EncodeToString(u.Output.PkScript),
			"height":      u.Height,
			"is_coinbase": u.IsCoinbase,
		})
	}
	return result, nil
}

func (r *RPCServer) handleGetAddress() (interface{}, *rpcError) {
	if r.wallet == nil {
		return nil, &rpcError{Code: -32000, Message: "no wallet loaded"}
	}
	return string(r.wallet.GetAddress()), nil
}

func (r *RPCServer) handleSendToAddress(params json.RawMessage) (interface{}, *rpcError) {
	if r.wallet == nil {
		return nil, &rpcError{Code: -32000, Message: "no wallet loaded"}
	}

	// params: [address, amount_nou] or [address, amount_nou, fee_nou]
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) < 2 {
		return nil, &rpcError{Code: -32602, Message: "params: [address, amount] or [address, amount, fee]"}
	}

	var addr string
	if err := json.Unmarshal(args[0], &addr); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid address param"}
	}
	var amount int64
	if err := json.Unmarshal(args[1], &amount); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid amount param"}
	}

	fee := int64(10000) // default 0.0001 NOUS
	if len(args) >= 3 {
		if err := json.Unmarshal(args[2], &fee); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid fee param"}
		}
	}

	transaction, err := r.wallet.CreateTransaction(
		crypto.Address(addr), amount, fee,
		r.chain.UTXOSet, r.chain.Height+1,
	)
	if err != nil {
		return nil, &rpcError{Code: -32000, Message: err.Error()}
	}

	// Validate before broadcasting.
	if err := tx.ValidateTransaction(transaction, r.chain.UTXOSet, r.chain.Height+1, r.chain.IsTestnet); err != nil {
		return nil, &rpcError{Code: -32000, Message: fmt.Sprintf("validate: %v", err)}
	}

	r.server.Mempool().Add(transaction)
	txID := transaction.TxID()
	raw := transaction.Serialize()
	r.server.BroadcastMessage(&network.MsgTx{Payload: raw})
	log.Printf("rpc: sendtoaddress %x (%d nou to %s) broadcast", txID[:8], amount, addr)

	return hex.EncodeToString(txID[:]), nil
}

// txToJSON converts a transaction to a JSON-friendly map.
// blockHeight is -1 for unconfirmed (mempool) transactions.
func txToJSON(t *tx.Transaction, blockHeight int64) map[string]interface{} {
	txID := t.TxID()
	inputs := make([]map[string]interface{}, len(t.Inputs))
	for i, in := range t.Inputs {
		inputs[i] = map[string]interface{}{
			"prev_txid": hex.EncodeToString(in.PrevOut.TxID[:]),
			"prev_index": in.PrevOut.Index,
		}
	}
	outputs := make([]map[string]interface{}, len(t.Outputs))
	for i, out := range t.Outputs {
		outputs[i] = map[string]interface{}{
			"value":  out.Amount,
			"script": hex.EncodeToString(out.PkScript),
		}
	}
	result := map[string]interface{}{
		"txid":    hex.EncodeToString(txID[:]),
		"version": t.Version,
		"inputs":  inputs,
		"outputs": outputs,
	}
	if blockHeight >= 0 {
		result["block_height"] = blockHeight
	} else {
		result["mempool"] = true
	}
	return result
}

func (r *RPCServer) handleGetTotalSupply() interface{} {
	height := r.chain.Height
	// Mainnet: genesis block has 0 NOUS (OP_RETURN), supply = height * 1 NOUS
	// Testnet: genesis block has 1 NOUS (P2PKH), supply = (height + 1) * 1 NOUS
	var expectedSupply int64
	if r.chain.IsTestnet {
		expectedSupply = int64(height+1) * 100000000
	} else {
		expectedSupply = int64(height) * 100000000
	}
	actualSupply := r.chain.UTXOSet.TotalSupply()
	return map[string]interface{}{
		"height":          height,
		"expected_supply": expectedSupply,
		"actual_supply":   actualSupply,
		"expected_nous":   float64(expectedSupply) / 100000000,
		"actual_nous":     float64(actualSupply) / 100000000,
		"match":           expectedSupply == actualSupply,
	}
}

func (r *RPCServer) handleGetWork(params json.RawMessage) (interface{}, *rpcError) {
	// params: [address, seed] or [address]
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) < 1 {
		return nil, &rpcError{Code: -32602, Message: "params: [address] or [address, seed]"}
	}

	var addr string
	if err := json.Unmarshal(args[0], &addr); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid address"}
	}
	seed := uint64(0)
	if len(args) >= 2 {
		json.Unmarshal(args[1], &seed)
	}

	pkh, err := crypto.DecodePubKeyHash(addr)
	if err != nil {
		return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("invalid address: %v", err)}
	}

	tip := r.chain.Tip
	height := r.chain.Height + 1
	diff := r.chain.GetDifficulty()
	prevHash := tip.Hash()

	// Generate SAT formula for this seed.
	satSeed := consensus.MakeSATSeed(prevHash, seed)
	formula := sat.GenerateFormula(satSeed, consensus.SATVariables, consensus.SATClausesRatio)

	// Format as DIMACS CNF.
	n := consensus.SATVariables
	m := len(formula)
	dimacs := fmt.Sprintf("p cnf %d %d\n", n, m)
	for _, clause := range formula {
		for _, lit := range clause {
			v := lit.Var + 1 // DIMACS is 1-indexed
			if lit.Neg {
				v = -v
			}
			dimacs += fmt.Sprintf("%d ", v)
		}
		dimacs += "0\n"
	}

	// Build header template for client-side PoW verification.
	// Build coinbase + merkle root (same as submitwork).
	mempoolTxs := r.server.Mempool().GetTopN(500)
	var validTxs []*tx.Transaction
	for _, t := range mempoolTxs {
		if err := tx.ValidateTransaction(t, r.chain.UTXOSet, height, r.chain.IsTestnet); err == nil {
			validTxs = append(validTxs, t)
		}
	}
	reward := consensus.BlockReward(height)
	var totalFees int64
	for _, t := range validTxs {
		var inputSum, outputSum int64
		for _, in := range t.Inputs {
			u := r.chain.UTXOSet.Get(in.PrevOut)
			if u != nil {
				inputSum += u.Output.Amount
			}
		}
		for _, out := range t.Outputs {
			outputSum += out.Amount
		}
		if inputSum > outputSum {
			totalFees += inputSum - outputSum
		}
	}
	coinbase := tx.NewCoinbaseTx(height, reward+totalFees, tx.CreateP2PKHLockScript(pkh), tx.ChainIDFor(r.chain.IsTestnet))
	allTxs := make([]*tx.Transaction, 0, 1+len(validTxs))
	allTxs = append(allTxs, coinbase)
	allTxs = append(allTxs, validTxs...)
	txIDs := make([]crypto.Hash, len(allTxs))
	for i, t := range allTxs {
		txIDs[i] = t.TxID()
	}
	merkleRoot := block.ComputeMerkleRoot(txIDs)

	now := uint32(time.Now().Unix())
	if now <= tip.Timestamp {
		now = tip.Timestamp + 1
	}

	diffBits := consensus.TargetToCompact(diff.PoWTarget)

	// Build 148-byte header template (seed=0, solution_hash=0 as placeholders).
	hdr := block.Header{
		Version:        1,
		PrevBlockHash:  prevHash,
		MerkleRoot:     merkleRoot,
		Timestamp:      now,
		DifficultyBits: diffBits,
		Seed:           0,
	}
	headerHex := hex.EncodeToString(hdr.Serialize())

	return map[string]interface{}{
		"height":          height,
		"prev_hash":       hex.EncodeToString(prevHash[:]),
		"difficulty_bits": diffBits,
		"seed":            seed,
		"n_vars":          n,
		"n_clauses":       m,
		"formula":         dimacs,
		"header_hex":      headerHex,
	}, nil
}

func (r *RPCServer) handleSubmitWork(params json.RawMessage) (interface{}, *rpcError) {
	// params: [address, seed, solution_binary_string, timestamp]
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) < 3 {
		return nil, &rpcError{Code: -32602, Message: "params: [address, seed, solution_bits, timestamp?]"}
	}

	var addr string
	if err := json.Unmarshal(args[0], &addr); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid address"}
	}
	var seed uint64
	if err := json.Unmarshal(args[1], &seed); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid seed"}
	}
	var solutionStr string
	if err := json.Unmarshal(args[2], &solutionStr); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid solution"}
	}
	// Optional timestamp from getwork.
	var clientTimestamp uint32
	if len(args) >= 4 {
		json.Unmarshal(args[3], &clientTimestamp)
	}

	// Parse solution binary string to Assignment.
	if len(solutionStr) != consensus.SATVariables {
		return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("solution must be %d bits, got %d", consensus.SATVariables, len(solutionStr))}
	}
	solution := make(sat.Assignment, consensus.SATVariables)
	for i, ch := range solutionStr {
		if ch == '1' {
			solution[i] = true
		} else if ch != '0' {
			return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("invalid character at position %d: %c", i, ch)}
		}
	}

	// Decode mining address.
	pkh, err := crypto.DecodePubKeyHash(addr)
	if err != nil {
		return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("invalid address: %v", err)}
	}

	// Verify SAT solution.
	tip := r.chain.Tip
	height := r.chain.Height + 1
	diff := r.chain.GetDifficulty()
	prevHash := tip.Hash()

	satSeed := consensus.MakeSATSeed(prevHash, seed)
	formula := sat.GenerateFormula(satSeed, consensus.SATVariables, consensus.SATClausesRatio)

	if !sat.Verify(formula, solution) {
		return nil, &rpcError{Code: -32000, Message: "SAT solution does not satisfy the formula"}
	}

	// Build coinbase and block.
	mempoolTxs := r.server.Mempool().GetTopN(500)
	var validTxs []*tx.Transaction
	for _, t := range mempoolTxs {
		if err := tx.ValidateTransaction(t, r.chain.UTXOSet, height, r.chain.IsTestnet); err == nil {
			validTxs = append(validTxs, t)
		}
	}

	reward := consensus.BlockReward(height)
	var totalFees int64
	for _, t := range validTxs {
		var inputSum, outputSum int64
		for _, in := range t.Inputs {
			u := r.chain.UTXOSet.Get(in.PrevOut)
			if u != nil {
				inputSum += u.Output.Amount
			}
		}
		for _, out := range t.Outputs {
			outputSum += out.Amount
		}
		if inputSum > outputSum {
			totalFees += inputSum - outputSum
		}
	}

	coinbase := tx.NewCoinbaseTx(height, reward+totalFees, tx.CreateP2PKHLockScript(pkh), tx.ChainIDFor(r.chain.IsTestnet))
	allTxs := make([]*tx.Transaction, 0, 1+len(validTxs))
	allTxs = append(allTxs, coinbase)
	allTxs = append(allTxs, validTxs...)

	txIDs := make([]crypto.Hash, len(allTxs))
	for i, t := range allTxs {
		txIDs[i] = t.TxID()
	}
	merkleRoot := block.ComputeMerkleRoot(txIDs)

	now := clientTimestamp
	if now == 0 {
		now = uint32(time.Now().Unix())
	}
	if now <= tip.Timestamp {
		now = tip.Timestamp + 1
	}
	// Don't allow timestamps too far in the future.
	maxAllowed := uint32(time.Now().Unix()) + 300
	if now > maxAllowed {
		now = uint32(time.Now().Unix())
		if now <= tip.Timestamp {
			now = tip.Timestamp + 1
		}
	}

	solBytes := sat.SerializeAssignment(solution)
	solHash := crypto.Sha256(solBytes)
	var utxoSetHash crypto.Hash

	hdr := block.Header{
		Version:         1,
		PrevBlockHash:   prevHash,
		MerkleRoot:      merkleRoot,
		Timestamp:       now,
		DifficultyBits:  consensus.TargetToCompact(diff.PoWTarget),
		Seed:            seed,
		SATSolutionHash: solHash,
		UTXOSetHash:     utxoSetHash,
	}

	// Check PoW.
	blockHash := hdr.Hash()
	if blockHash.Compare(diff.PoWTarget) > 0 {
		return nil, &rpcError{Code: -32000, Message: "block hash does not meet difficulty target (try different seed)"}
	}

	blk := &block.Block{
		Header:       hdr,
		Transactions: allTxs,
		SATSolution:  solution,
	}

	// Apply to chain.
	if err := r.chain.AddBlock(blk); err != nil {
		return nil, &rpcError{Code: -32000, Message: fmt.Sprintf("add block: %v", err)}
	}

	if err := r.store.SaveBlock(blk, height); err != nil {
		log.Printf("rpc: submitwork save block %d: %v", height, err)
	}
	tipHash := blk.Header.Hash()
	r.store.SaveChainTip(storage.ChainTip{Hash: tipHash, Height: height})
	r.server.SetBlockHeight(height)
	r.server.Mempool().RemoveConfirmed(blk.Transactions)

	// Broadcast.
	payload, err := network.EncodeBlock(blk)
	if err != nil {
		log.Printf("rpc: submitwork encode block: %v", err)
	} else {
		r.server.BroadcastMessage(&network.MsgBlock{Payload: payload})
	}

	log.Printf("rpc: submitwork block %d hash=%x from %s", height, tipHash[:8], addr)
	return map[string]interface{}{
		"height":     height,
		"block_hash": hex.EncodeToString(tipHash[:]),
	}, nil
}
