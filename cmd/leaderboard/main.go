// leaderboard is a background service that scans the NOUS blockchain,
// writes leaderboard/explorer JSON files, and serves an HTTP API for
// address transaction history. All index data is persisted in SQLite
// so restarts only need to scan new blocks.
package main

import (
	"database/sql"
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

	_ "modernc.org/sqlite"
)

// --- RPC client ---

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

// --- Node count ---

var seedRPCURLs = []string{
	"http://80.78.26.7:8332/rpc",
	"http://80.78.25.211:8332/rpc",
	"http://80.78.26.244:8332/rpc",
}

var seedHosts = map[string]bool{
	"80.78.26.7": true, "80.78.25.211": true, "80.78.26.244": true,
	"seed1.nouschain.org": true, "seed2.nouschain.org": true, "seed3.nouschain.org": true,
}

var (
	cachedNodeCount int
	cachedNodeTime  time.Time
)

const nodeCountCacheTTL = 5 * time.Minute

func countNetworkNodes() int {
	type peerInfo struct {
		Addr string `json:"addr"`
	}
	uniqueIPs := make(map[string]bool)

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

	type result struct{ peers []peerInfo }
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

	nodeCount := 3
	for ip := range uniqueIPs {
		if !seedHosts[ip] {
			nodeCount++
		}
	}
	if nodeCount < 1 {
		nodeCount = 1
	}
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

type addrTx struct {
	TxID      string `json:"txid"`
	Height    int    `json:"height"`
	Timestamp int64  `json:"timestamp"`
	Value     int64  `json:"value"`
	Direction string `json:"direction"`
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

// --- SQLite storage ---

var db *sql.DB

func initDB(dbPath string) error {
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	// Performance tuning.
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA synchronous=NORMAL")

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS miners (
			address TEXT PRIMARY KEY,
			blocks INTEGER NOT NULL DEFAULT 0,
			last_ts INTEGER NOT NULL DEFAULT 0,
			last_height INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS diff_points (
			height INTEGER PRIMARY KEY,
			difficulty INTEGER NOT NULL,
			timestamp INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS addr_txs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			address TEXT NOT NULL,
			txid TEXT NOT NULL,
			height INTEGER NOT NULL,
			timestamp INTEGER NOT NULL,
			value INTEGER NOT NULL,
			direction TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_addr ON addr_txs(address);
		CREATE TABLE IF NOT EXISTS outputs (
			key TEXT PRIMARY KEY,
			address TEXT NOT NULL,
			value INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

func dbGetLastScan() int {
	var val string
	err := db.QueryRow("SELECT value FROM meta WHERE key='last_scan'").Scan(&val)
	if err != nil {
		return -1
	}
	var n int
	fmt.Sscanf(val, "%d", &n)
	return n
}

func dbSetLastScan(height int) {
	db.Exec("INSERT OR REPLACE INTO meta(key,value) VALUES('last_scan', ?)", fmt.Sprintf("%d", height))
}

// --- Block scanning ---

type blockResult struct {
	Height       int      `json:"height"`
	Timestamp    int64    `json:"timestamp"`
	Difficulty   int64    `json:"difficulty"`
	MinerAddress string   `json:"miner_address"`
	TxCount      int      `json:"tx_count"`
	Transactions []string `json:"transactions"`
}

const batchSize = 50

var mu sync.Mutex // protects DB writes during scan batches

func scanNewBlocks(height int) error {
	lastScan := dbGetLastScan()
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

		// 1. Fetch blocks in parallel.
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

		// 2. Only fetch tx details for blocks with real transactions (tx_count > 1).
		type txKey struct {
			blockIdx int
			txid     string
		}
		var txKeys []txKey
		for i, b := range blocks {
			if blockErrs[i] != nil || b.TxCount <= 1 {
				continue
			}
			for _, txid := range b.Transactions {
				txKeys = append(txKeys, txKey{blockIdx: i, txid: txid})
			}
		}

		txResults := make([]txData, len(txKeys))
		txErrs := make([]error, len(txKeys))
		sem := make(chan struct{}, 8)
		for k, tk := range txKeys {
			wg.Add(1)
			go func(k int, txid string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				raw, err := rpc("gettx", txid)
				if err != nil {
					txErrs[k] = err
					return
				}
				json.Unmarshal(raw, &txResults[k])
			}(k, tk.txid)
		}
		wg.Wait()

		blockTxs := map[int][]txData{}
		for k, tk := range txKeys {
			if txErrs[k] == nil {
				blockTxs[tk.blockIdx] = append(blockTxs[tk.blockIdx], txResults[k])
			}
		}

		// 3. Process blocks sequentially, write to SQLite in a transaction.
		mu.Lock()
		tx, err := db.Begin()
		if err != nil {
			mu.Unlock()
			return fmt.Errorf("begin tx: %w", err)
		}

		stmtMiner, _ := tx.Prepare(`INSERT INTO miners(address,blocks,last_ts,last_height) VALUES(?,1,?,?)
			ON CONFLICT(address) DO UPDATE SET blocks=blocks+1, last_ts=MAX(last_ts,excluded.last_ts), last_height=MAX(last_height,excluded.last_height)`)
		stmtDiff, _ := tx.Prepare("INSERT OR IGNORE INTO diff_points(height,difficulty,timestamp) VALUES(?,?,?)")
		stmtAddrTx, _ := tx.Prepare("INSERT INTO addr_txs(address,txid,height,timestamp,value,direction) VALUES(?,?,?,?,?,?)")
		stmtOutput, _ := tx.Prepare("INSERT OR REPLACE INTO outputs(key,address,value) VALUES(?,?,?)")

		for i, b := range blocks {
			if blockErrs[i] != nil {
				continue
			}

			stmtDiff.Exec(b.Height, b.Difficulty, b.Timestamp)

			if b.MinerAddress != "" {
				stmtMiner.Exec(b.MinerAddress, b.Timestamp, b.Height)
			}

			// Address index.
			if b.TxCount <= 1 && b.MinerAddress != "" && len(b.Transactions) == 1 {
				// Coinbase-only: record directly.
				coinbaseTxID := b.Transactions[0]
				var reward int64
				if b.Height > 0 {
					reward = 100000000
				}
				if reward > 0 {
					key := fmt.Sprintf("%s:0", coinbaseTxID)
					stmtOutput.Exec(key, b.MinerAddress, reward)
					stmtAddrTx.Exec(b.MinerAddress, coinbaseTxID, b.Height, b.Timestamp, reward, "in")
				}
			} else {
				for _, t := range blockTxs[i] {
					for vout, out := range t.Outputs {
						addr := scriptToAddress(out.Script)
						if addr == "" {
							continue
						}
						key := fmt.Sprintf("%s:%d", t.TxID, vout)
						stmtOutput.Exec(key, addr, out.Value)
						stmtAddrTx.Exec(addr, t.TxID, b.Height, b.Timestamp, out.Value, "in")
					}
					for _, inp := range t.Inputs {
						if inp.PrevTxID == zeroCoinbasePrev {
							continue
						}
						key := fmt.Sprintf("%s:%d", inp.PrevTxID, inp.PrevIndex)
						var refAddr string
						var refValue int64
						err := tx.QueryRow("SELECT address,value FROM outputs WHERE key=?", key).Scan(&refAddr, &refValue)
						if err != nil {
							continue
						}
						stmtAddrTx.Exec(refAddr, t.TxID, b.Height, b.Timestamp, refValue, "out")
					}
				}
			}
		}

		stmtMiner.Close()
		stmtDiff.Close()
		stmtAddrTx.Close()
		stmtOutput.Close()

		tx.Exec("INSERT OR REPLACE INTO meta(key,value) VALUES('last_scan', ?)", fmt.Sprintf("%d", end))
		tx.Commit()
		mu.Unlock()
	}

	log.Printf("leaderboard: scan complete, height=%d", height)
	return nil
}

// --- Build JSON outputs ---

func buildJSON(height int) leaderboardData {
	// Load miners from DB.
	rows, err := db.Query("SELECT address,blocks,last_ts,last_height FROM miners")
	if err != nil {
		return leaderboardData{UpdatedAt: time.Now().Unix(), Height: height}
	}
	defer rows.Close()

	var miners []minerStat
	for rows.Next() {
		var m minerStat
		rows.Scan(&m.Address, &m.Blocks, &m.LastTs, &m.LastHeight)
		miners = append(miners, m)
	}

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

// --- HTTP API ---

func startAPI(listenAddr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/address/", handleAddrHistory)
	log.Printf("explorer API listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatalf("explorer API: %v", err)
	}
}

func handleAddrHistory(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/address/")
	addr := strings.TrimSuffix(path, "/txs")
	addr = strings.TrimSuffix(addr, "/")

	if addr == "" || !strings.HasPrefix(addr, "nous1") {
		http.Error(w, `{"error":"invalid address"}`, http.StatusBadRequest)
		return
	}

	rows, err := db.Query(
		"SELECT txid,height,timestamp,value,direction FROM addr_txs WHERE address=? ORDER BY height DESC",
		addr)
	if err != nil {
		http.Error(w, `[]`, 200)
		return
	}
	defer rows.Close()

	var txs []addrTx
	for rows.Next() {
		var t addrTx
		rows.Scan(&t.TxID, &t.Height, &t.Timestamp, &t.Value, &t.Direction)
		txs = append(txs, t)
	}
	if txs == nil {
		txs = []addrTx{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(txs)
}

// --- Main ---

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

	// Difficulty chart (last 200 points from DB).
	dRows, err := db.Query("SELECT height,difficulty,timestamp FROM diff_points ORDER BY height DESC LIMIT 200")
	if err == nil {
		var pts []diffPoint
		for dRows.Next() {
			var p diffPoint
			dRows.Scan(&p.Height, &p.Difficulty, &p.Timestamp)
			pts = append(pts, p)
		}
		dRows.Close()
		// Reverse to ascending order.
		for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
			pts[i], pts[j] = pts[j], pts[i]
		}
		diffData := diffChartData{UpdatedAt: time.Now().Unix(), Points: pts}
		baseDir := outFile[:len(outFile)-len("leaderboard.json")]
		writeJSON(baseDir+"difficulty.json", diffData)
	}

	// Recent blocks.
	recentData := fetchRecentBlocks(height)
	baseDir := outFile[:len(outFile)-len("leaderboard.json")]
	writeJSON(baseDir+"recentblocks.json", recentData)

	// Stats.
	var addrCount, txCount int
	db.QueryRow("SELECT COUNT(DISTINCT address) FROM addr_txs").Scan(&addrCount)
	db.QueryRow("SELECT COUNT(*) FROM addr_txs").Scan(&txCount)
	log.Printf("leaderboard: updated height=%d miners=%d addresses=%d tx_records=%d",
		height, data.TotalMiners, addrCount, txCount)
}

func main() {
	node := flag.String("node", "http://localhost:8332/rpc", "Node RPC URL")
	out := flag.String("out", "/var/www/nous/leaderboard.json", "Output JSON file")
	interval := flag.Duration("interval", 30*time.Second, "Update interval")
	apiAddr := flag.String("api", ":8081", "Explorer API listen address")
	dbPath := flag.String("db", "/var/www/nous/leaderboard.db", "SQLite database path")
	flag.Parse()

	nodeURL = *node

	if err := initDB(*dbPath); err != nil {
		log.Fatalf("init db: %v", err)
	}

	lastScan := dbGetLastScan()
	log.Printf("leaderboard starting: node=%s out=%s api=%s db=%s last_scan=%d",
		nodeURL, *out, *apiAddr, *dbPath, lastScan)

	go startAPI(*apiAddr)

	update(*out)
	for range time.Tick(*interval) {
		update(*out)
	}
}
