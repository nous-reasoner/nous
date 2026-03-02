package node

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"

	"nous/consensus"
	"nous/crypto"
	"nous/network"
	"nous/storage"
	"nous/tx"
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
	chain  *consensus.ChainState
	server *network.Server
	store  *storage.BlockStore
	reasoner *Reasoner
	http   *http.Server
	addr   string // actual bound address after Start
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
	return map[string]interface{}{
		"hash":         hex.EncodeToString(hash[:]),
		"height":       height,
		"version":      blk.Header.Version,
		"timestamp":    blk.Header.Timestamp,
		"prev_hash":    hex.EncodeToString(blk.Header.PrevBlockHash[:]),
		"merkle_root":  hex.EncodeToString(blk.Header.MerkleRoot[:]),
		"difficulty":   blk.Header.DifficultyBits,
		"seed":         blk.Header.Seed,
		"tx_count":     len(blk.Transactions),
		"transactions": txIDs,
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
	if err := tx.ValidateTransaction(transaction, r.chain.UTXOSet, r.chain.Height+1); err != nil {
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
	balance := r.chain.UTXOSet.GetBalance(pkh)
	return balance, nil
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
	result := make([]map[string]interface{}, 0, len(utxos))
	for _, u := range utxos {
		result = append(result, map[string]interface{}{
			"txid":   hex.EncodeToString(u.OutPoint.TxID[:]),
			"index":  u.OutPoint.Index,
			"value":  u.Output.Amount,
			"script": hex.EncodeToString(u.Output.PkScript),
			"height": u.Height,
		})
	}
	return result, nil
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
