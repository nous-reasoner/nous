// leaderboard is a background service that scans the NOUS blockchain and
// writes a leaderboard.json file for the block explorer frontend.
// It also builds an in-memory address→transaction index and serves
// an HTTP API for the explorer to query address history.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"nous/crypto"
)

type rpcRequest struct {
	Method string `json:"method"`
	Params []any  `json:"params"`
	ID     int    `json:"id"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

var nodeURL string

func rpc(method string, params ...any) (json.RawMessage, error) {
	body, _ := json.Marshal(rpcRequest{Method: method, Params: params, ID: 1})
	resp, err := http.Post(nodeURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var r rpcResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, &rpcErr{r.Error.Message}
	}
	return r.Result, nil
}

type rpcErr struct{ msg string }

func (e *rpcErr) Error() string { return e.msg }

// rpcTo calls an RPC method on a specific node URL.
func rpcTo(url, method string, params ...any) (json.RawMessage, error) {
	body, _ := json.Marshal(rpcRequest{Method: method, Params: params, ID: 1})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var r rpcResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, &rpcErr{r.Error.Message}
	}
	return r.Result, nil
}

// seedRPCURLs lists all seed node RPC endpoints for cross-node queries.
var seedRPCURLs = []string{
	"http://80.78.26.7:8332/rpc",
	"http://80.78.25.211:8332/rpc",
	"http://80.78.26.244:8332/rpc",
}

// seedHosts contains all known seed node IPs and hostnames to avoid double-counting.
var seedHosts = map[string]bool{
	"80.78.26.7": true, "80.78.25.211": true, "80.78.26.244": true,
	"seed1.nouschain.org": true, "seed2.nouschain.org": true, "seed3.nouschain.org": true,
}

// cached node count to smooth over transient RPC failures
var (
	cachedNodeCount int
	cachedNodeTime  time.Time
)

const nodeCountCacheTTL = 5 * time.Minute

// countNetworkNodes queries all seed nodes' getpeerinfo, deduplicates by IP,
// and returns total unique node count (seeds + external).
// Uses a 5-minute cache: if the fresh count is lower due to RPC failures,
// the cached value is returned until it expires.
func countNetworkNodes() int {
	type peerInfo struct {
		Addr string `json:"addr"`
	}

	uniqueIPs := make(map[string]bool)

	// Query local node first (always fast, no network hop).
	if raw, err := rpc("getpeerinfo"); err == nil {
		var peers []peerInfo
		if json.Unmarshal(raw, &peers) == nil {
			for _, p := range peers {
				host := p.Addr
				if idx := strings.LastIndex(host, ":"); idx > 0 {
					host = host[:idx]
				}
				uniqueIPs[host] = true
			}
		}
	}

	// Query remote seed nodes in parallel.
	type result struct {
		peers []peerInfo
	}
	results := make([]result, len(seedRPCURLs))
	var wg sync.WaitGroup
	for i, seedURL := range seedRPCURLs {
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			raw, err := rpcTo(url, "getpeerinfo")
			if err != nil {
				return
			}
			var peers []peerInfo
			if json.Unmarshal(raw, &peers) == nil {
				results[i] = result{peers: peers}
			}
		}(i, seedURL)
	}
	wg.Wait()

	for _, r := range results {
		for _, p := range r.peers {
			host := p.Addr
			if idx := strings.LastIndex(host, ":"); idx > 0 {
				host = host[:idx]
			}
			uniqueIPs[host] = true
		}
	}

	// Count: 3 seed nodes + unique external IPs.
	nodeCount := 3 // 3 seed nodes
	for ip := range uniqueIPs {
		if !seedHosts[ip] {
			nodeCount++
		}
	}
	if nodeCount < 1 {
		nodeCount = 1
	}

	// Cache: only update if fresh count is >= cached, or cache expired.
	if nodeCount >= cachedNodeCount || time.Since(cachedNodeTime) > nodeCountCacheTTL {
		cachedNodeCount = nodeCount
		cachedNodeTime = time.Now()
	}
	return cachedNodeCount
}

// --- Data types ---

type minerStat struct {
	Address    string  `json:"address"`
	Blocks     int     `json:"blocks"`
	TotalNous  float64 `json:"total_nous"`
	LastTs     int64   `json:"last_ts"`
	LastHeight int     `json:"last_height"`
}

type leaderboardData struct {
	UpdatedAt   int64       `json:"updated_at"`
	Height      int         `json:"height"`
	TotalMiners int         `json:"total_miners"`
	Miners      []minerStat `json:"miners"`
}

type diffPoint struct {
	Height     int   `json:"h"`
	Difficulty int64 `json:"d"`
	Timestamp  int64 `json:"t"`
}

type diffChartData struct {
	UpdatedAt int64       `json:"updated_at"`
	Points    []diffPoint `json:"points"`
}

// --- Address index ---

type addrTx struct {
	TxID      string `json:"txid"`
	Height    int    `json:"height"`
	Timestamp int64  `json:"timestamp"`
	Value     int64  `json:"value"`
	Direction string `json:"direction"` // "in" = received, "out" = sent
}

type outputRef struct {
	Address string
	Value   int64
}

// gettx response shape
type txData struct {
	TxID    string `json:"txid"`
	Inputs  []struct {
		PrevTxID  string `json:"prev_txid"`
		PrevIndex int    `json:"prev_index"`
	} `json:"inputs"`
	Outputs []struct {
		Value  int64  `json:"value"`
		Script string `json:"script"`
	} `json:"outputs"`
}

const zeroCoinbasePrev = "0000000000000000000000000000000000000000000000000000000000000000"

// scriptToAddress decodes a P2PKH script (76a914{20-byte-pkh}88ac) to a bech32m address.
func scriptToAddress(scriptHex string) string {
	if len(scriptHex) != 50 || !strings.HasPrefix(scriptHex, "76a914") || !strings.HasSuffix(scriptHex, "88ac") {
		return ""
	}
	pkh, err := hex.DecodeString(scriptHex[6:46])
	if err != nil || len(pkh) != 20 {
		return ""
	}
	return crypto.PubKeyHashToBech32mAddress(pkh)
}

// --- State ---

var (
	mu         sync.Mutex
	minerMap   = map[string]*minerStat{}
	diffPoints []diffPoint
	lastScan   = -1
	addrIndex  = map[string][]addrTx{}
	outputMap  = map[string]outputRef{} // "txid:vout" → ref
)

const batchSize = 50

// --- Block scanning ---

type blockResult struct {
	Height       int      `json:"height"`
	Timestamp    int64    `json:"timestamp"`
	Difficulty   int64    `json:"difficulty"`
	MinerAddress string   `json:"miner_address"`
	TxCount      int      `json:"tx_count"`
	Transactions []string `json:"transactions"`
}

func scanNewBlocks(height int) error {
	from := lastScan + 1
	if from > height {
		return nil
	}
	log.Printf("leaderboard: scanning blocks %d..%d", from, height)

	for start := from; start <= height; start += batchSize {
		end := start + batchSize - 1
		if end > height {
			end = height
		}

		// 1. Fetch block headers in parallel.
		blocks := make([]blockResult, end-start+1)
		blockErrs := make([]error, end-start+1)
		var wg sync.WaitGroup
		for h := start; h <= end; h++ {
			wg.Add(1)
			go func(h, i int) {
				defer wg.Done()
				raw, err := rpc("getblock", h)
				if err != nil {
					blockErrs[i] = err
					return
				}
				json.Unmarshal(raw, &blocks[i])
			}(h, h-start)
		}
		wg.Wait()

		// 2. Collect all txids from this batch and fetch them in parallel.
		type txKey struct {
			blockIdx int
			txIdx    int
			txid     string
		}
		var allTxKeys []txKey
		for i, b := range blocks {
			if blockErrs[i] != nil {
				continue
			}
			for j, txid := range b.Transactions {
				allTxKeys = append(allTxKeys, txKey{blockIdx: i, txIdx: j, txid: txid})
			}
		}

		txResults := make([]txData, len(allTxKeys))
		txErrs := make([]error, len(allTxKeys))
		for k, tk := range allTxKeys {
			wg.Add(1)
			go func(k int, txid string) {
				defer wg.Done()
				raw, err := rpc("gettx", txid)
				if err != nil {
					txErrs[k] = err
					return
				}
				json.Unmarshal(raw, &txResults[k])
			}(k, tk.txid)
		}
		wg.Wait()

		// Build a map: blockIdx → []txData (preserving tx order within block)
		blockTxs := map[int][]txData{}
		for k, tk := range allTxKeys {
			if txErrs[k] != nil {
				continue
			}
			blockTxs[tk.blockIdx] = append(blockTxs[tk.blockIdx], txResults[k])
		}

		// 3. Process blocks sequentially (for outputMap ordering).
		mu.Lock()
		for i, b := range blocks {
			if blockErrs[i] != nil {
				continue
			}

			// Difficulty chart data.
			diffPoints = append(diffPoints, diffPoint{
				Height:     b.Height,
				Difficulty: b.Difficulty,
				Timestamp:  b.Timestamp,
			})

			// Miner leaderboard.
			if b.MinerAddress != "" {
				s, ok := minerMap[b.MinerAddress]
				if !ok {
					s = &minerStat{Address: b.MinerAddress}
					minerMap[b.MinerAddress] = s
				}
				s.Blocks++
				if b.Timestamp > s.LastTs {
					s.LastTs = b.Timestamp
					s.LastHeight = b.Height
				}
			}

			// Address index: process this block's transactions.
			for _, t := range blockTxs[i] {
				// Outputs first (populate outputMap before resolving inputs).
				for vout, out := range t.Outputs {
					addr := scriptToAddress(out.Script)
					if addr == "" {
						continue
					}
					key := fmt.Sprintf("%s:%d", t.TxID, vout)
					outputMap[key] = outputRef{Address: addr, Value: out.Value}
					addrIndex[addr] = append(addrIndex[addr], addrTx{
						TxID:      t.TxID,
						Height:    b.Height,
						Timestamp: b.Timestamp,
						Value:     out.Value,
						Direction: "in",
					})
				}

				// Inputs (skip coinbase).
				for _, inp := range t.Inputs {
					if inp.PrevTxID == zeroCoinbasePrev {
						continue
					}
					key := fmt.Sprintf("%s:%d", inp.PrevTxID, inp.PrevIndex)
					ref, ok := outputMap[key]
					if !ok {
						continue
					}
					addrIndex[ref.Address] = append(addrIndex[ref.Address], addrTx{
						TxID:      t.TxID,
						Height:    b.Height,
						Timestamp: b.Timestamp,
						Value:     ref.Value,
						Direction: "out",
					})
				}
			}
		}
		mu.Unlock()
	}

	lastScan = height
	return nil
}

// --- Build JSON outputs ---

func buildJSON(height int) leaderboardData {
	mu.Lock()
	miners := make([]minerStat, 0, len(minerMap))
	for _, s := range minerMap {
		miners = append(miners, *s)
	}
	mu.Unlock()

	// Fetch actual balance for each miner.
	var wg sync.WaitGroup
	for i := range miners {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raw, err := rpc("getbalance", miners[i].Address)
			if err != nil {
				return
			}
			var bal struct {
				Balance  int64 `json:"balance"`
				Immature int64 `json:"immature"`
			}
			if json.Unmarshal(raw, &bal) == nil {
				miners[i].TotalNous = float64(bal.Balance+bal.Immature) / 1e8
			}
		}(i)
	}
	wg.Wait()

	sort.Slice(miners, func(i, j int) bool {
		return miners[i].TotalNous > miners[j].TotalNous
	})
	return leaderboardData{
		UpdatedAt:   time.Now().Unix(),
		Height:      height,
		TotalMiners: len(miners),
		Miners:      miners,
	}
}

func writeJSON(path string, v any) error {
	out, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// --- Recent blocks ---

type recentBlock struct {
	Height       int      `json:"height"`
	Hash         string   `json:"hash"`
	Timestamp    int64    `json:"timestamp"`
	Difficulty   int64    `json:"difficulty"`
	Seed         uint64   `json:"seed"`
	TxCount      int      `json:"tx_count"`
	MinerAddress string   `json:"miner_address"`
	Transactions []string `json:"transactions,omitempty"`
}

type recentBlocksData struct {
	UpdatedAt      int64         `json:"updated_at"`
	Height         int           `json:"height"`
	DifficultyBits int64         `json:"difficulty_bits"`
	Peers          int           `json:"peers"`
	Blocks         []recentBlock `json:"blocks"`
}

func fetchRecentBlocks(height int) recentBlocksData {
	count := 20
	start := height
	if start < 0 {
		start = 0
	}
	end := start - count + 1
	if end < 0 {
		end = 0
	}

	type result struct {
		idx   int
		block recentBlock
		err   error
	}

	results := make([]result, start-end+1)
	var wg sync.WaitGroup
	for h := start; h >= end; h-- {
		wg.Add(1)
		go func(h, i int) {
			defer wg.Done()
			raw, err := rpc("getblock", h)
			if err != nil {
				results[i] = result{idx: i, err: err}
				return
			}
			var b recentBlock
			json.Unmarshal(raw, &b)
			results[i] = result{idx: i, block: b}
		}(h, start-h)
		wg.Wait()
	}

	blocks := make([]recentBlock, 0, len(results))
	for _, r := range results {
		if r.err == nil {
			blocks = append(blocks, r.block)
		}
	}

	peers := countNetworkNodes()

	var diffBits int64
	if raw, err := rpc("getmininginfo"); err == nil {
		var info struct {
			DifficultyBits int64 `json:"difficulty_bits"`
		}
		if json.Unmarshal(raw, &info) == nil {
			diffBits = info.DifficultyBits
		}
	}

	return recentBlocksData{
		UpdatedAt:      time.Now().Unix(),
		Height:         height,
		DifficultyBits: diffBits,
		Peers:          peers,
		Blocks:         blocks,
	}
}

// --- HTTP API for address history ---

func startAPI(listenAddr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/address/", handleAddrHistory)
	log.Printf("explorer API listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatalf("explorer API: %v", err)
	}
}

func handleAddrHistory(w http.ResponseWriter, r *http.Request) {
	// URL: /address/{addr}/txs
	path := strings.TrimPrefix(r.URL.Path, "/address/")
	addr := strings.TrimSuffix(path, "/txs")
	addr = strings.TrimSuffix(addr, "/")

	if addr == "" || !strings.HasPrefix(addr, "nous1") {
		http.Error(w, `{"error":"invalid address"}`, http.StatusBadRequest)
		return
	}

	mu.Lock()
	txs := addrIndex[addr]
	result := make([]addrTx, len(txs))
	copy(result, txs)
	mu.Unlock()

	// Most recent first.
	sort.Slice(result, func(i, j int) bool {
		return result[i].Height > result[j].Height
	})

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(result)
}

// --- Main update loop ---

func update(outFile string) {
	raw, err := rpc("getblockcount")
	if err != nil {
		log.Printf("leaderboard: getblockcount: %v", err)
		return
	}
	var height int
	if err := json.Unmarshal(raw, &height); err != nil {
		log.Printf("leaderboard: parse height: %v", err)
		return
	}

	if err := scanNewBlocks(height); err != nil {
		log.Printf("leaderboard: scan: %v", err)
		return
	}

	data := buildJSON(height)
	if err := writeJSON(outFile, data); err != nil {
		log.Printf("leaderboard: write %s: %v", outFile, err)
		return
	}

	// Write difficulty chart JSON (last 200 points).
	mu.Lock()
	pts := diffPoints
	if len(pts) > 200 {
		pts = pts[len(pts)-200:]
	}
	diffData := diffChartData{UpdatedAt: time.Now().Unix(), Points: pts}
	mu.Unlock()

	baseDir := outFile[:len(outFile)-len("leaderboard.json")]
	diffFile := baseDir + "difficulty.json"
	if err := writeJSON(diffFile, diffData); err != nil {
		log.Printf("leaderboard: write %s: %v", diffFile, err)
	}

	// Write recent blocks JSON for explorer homepage.
	recentData := fetchRecentBlocks(height)
	recentFile := baseDir + "recentblocks.json"
	if err := writeJSON(recentFile, recentData); err != nil {
		log.Printf("leaderboard: write %s: %v", recentFile, err)
	}

	mu.Lock()
	addrCount := len(addrIndex)
	txCount := 0
	for _, txs := range addrIndex {
		txCount += len(txs)
	}
	mu.Unlock()

	log.Printf("leaderboard: updated height=%d miners=%d addresses=%d tx_records=%d",
		height, data.TotalMiners, addrCount, txCount)
}

func main() {
	node := flag.String("node", "http://localhost:8332/rpc", "Node RPC URL")
	out := flag.String("out", "/var/www/nous/leaderboard.json", "Output JSON file")
	interval := flag.Duration("interval", 30*time.Second, "Update interval")
	apiAddr := flag.String("api", ":8081", "Explorer API listen address")
	flag.Parse()

	nodeURL = *node

	log.Printf("leaderboard starting: node=%s out=%s interval=%s api=%s",
		nodeURL, *out, *interval, *apiAddr)

	// Start HTTP API server in background.
	go startAPI(*apiAddr)

	// Run immediately on start, then on interval.
	update(*out)
	for range time.Tick(*interval) {
		update(*out)
	}
}
