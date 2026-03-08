//go:build js && wasm

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"syscall/js"
	"time"

	"nous/sat"
)

const (
	satVariables   = 256
	satClausesRatio = 3.85
	satSolveTimeout = 100 * time.Millisecond
	seedsPerRound   = 10000
)

var (
	mining   bool
	stopCh   chan struct{}
	mu       sync.Mutex
	stats    miningStats
)

type miningStats struct {
	SATSolved   int     `json:"sat_solved"`
	SeedsTried  int     `json:"seeds_tried"`
	BlocksMined int     `json:"blocks_mined"`
	HashRate    float64 `json:"hash_rate"`
	Running     bool    `json:"running"`
	Height      uint64  `json:"height"`
	Elapsed     float64 `json:"elapsed"`
}

func main() {
	miner := js.Global().Get("Object").New()
	miner.Set("start", js.FuncOf(startMining))
	miner.Set("stop", js.FuncOf(stopMining))
	miner.Set("getStats", js.FuncOf(getStats))
	js.Global().Set("nousMiner", miner)

	jsLog("NOUS WebAssembly Miner loaded")

	// Keep Go running.
	select {}
}

func jsLog(msg string) {
	js.Global().Call("postMinerLog", msg)
}

func startMining(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return "error: need nodeURL and address"
	}
	nodeURL := args[0].String()
	address := args[1].String()

	mu.Lock()
	if mining {
		mu.Unlock()
		return "already mining"
	}
	mining = true
	stopCh = make(chan struct{})
	stats = miningStats{Running: true}
	mu.Unlock()

	go mineLoop(nodeURL, address)
	return "started"
}

func stopMining(this js.Value, args []js.Value) interface{} {
	mu.Lock()
	defer mu.Unlock()
	if !mining {
		return "not mining"
	}
	close(stopCh)
	mining = false
	stats.Running = false
	jsLog("Mining stopped")
	return "stopped"
}

func getStats(this js.Value, args []js.Value) interface{} {
	mu.Lock()
	defer mu.Unlock()
	b, _ := json.Marshal(stats)
	return string(b)
}

func mineLoop(nodeURL, address string) {
	defer func() {
		mu.Lock()
		mining = false
		stats.Running = false
		mu.Unlock()
	}()

	jsLog(fmt.Sprintf("Mining started: node=%s", nodeURL))
	globalStart := time.Now()

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		err := mineRound(nodeURL, address, globalStart)
		if err != nil {
			jsLog(fmt.Sprintf("Error: %v", err))
			// Wait before retry.
			select {
			case <-stopCh:
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func mineRound(nodeURL, address string, globalStart time.Time) error {
	// getwork
	workRaw, err := rpcCall(nodeURL, "getwork", []interface{}{address, uint64(0)})
	if err != nil {
		return fmt.Errorf("getwork: %w", err)
	}

	var work struct {
		Height    uint64 `json:"height"`
		PrevHash  string `json:"prev_hash"`
		DiffBits  uint32 `json:"difficulty_bits"`
		HeaderHex string `json:"header_hex"`
	}
	if err := json.Unmarshal(workRaw, &work); err != nil {
		return fmt.Errorf("parse work: %w", err)
	}

	mu.Lock()
	stats.Height = work.Height
	mu.Unlock()

	jsLog(fmt.Sprintf("Mining block %d (diff: 0x%08x)", work.Height, work.DiffBits))

	headerTemplate, err := hex.DecodeString(work.HeaderHex)
	if err != nil || len(headerTemplate) != 148 {
		return fmt.Errorf("invalid header template")
	}

	prevHashBytes, err := hex.DecodeString(work.PrevHash)
	if err != nil || len(prevHashBytes) != 32 {
		return fmt.Errorf("invalid prev_hash")
	}
	var prevHash [32]byte
	copy(prevHash[:], prevHashBytes)

	target := compactToTarget(work.DiffBits)
	headerTimestamp := binary.LittleEndian.Uint32(headerTemplate[68:72])
	satSolved := 0
	roundStart := time.Now()

	for seed := uint64(0); seed < seedsPerRound; seed++ {
		select {
		case <-stopCh:
			return nil
		default:
		}

		satSeed := makeSATSeed(prevHash, seed)
		formula := sat.GenerateFormula(satSeed, satVariables, satClausesRatio)

		solution, err := sat.ProbSATSolve(formula, satVariables, satSolveTimeout)
		if err != nil {
			continue
		}

		if !sat.Verify(formula, solution) {
			continue
		}
		satSolved++

		// Compute solution hash.
		solBytes := sat.SerializeAssignment(solution)
		solHash := sha256.Sum256(solBytes)

		// Build header and check PoW.
		header := make([]byte, 148)
		copy(header, headerTemplate)
		binary.LittleEndian.PutUint64(header[76:84], seed)
		copy(header[84:116], solHash[:])

		blockHash := doubleSha256(header)
		if !meetsTarget(blockHash, target) {
			if satSolved%20 == 0 {
				elapsed := time.Since(roundStart).Seconds()
				rate := float64(satSolved) / elapsed
				mu.Lock()
				stats.SATSolved += 0 // don't double count, updated at end
				stats.HashRate = rate
				stats.Elapsed = time.Since(globalStart).Seconds()
				mu.Unlock()
				jsLog(fmt.Sprintf("Progress: %d SAT solved (%.1f/s), seed=%d", satSolved, rate, seed))
			}
			continue
		}

		// PoW met — submit!
		solutionStr := assignmentToString(solution)
		jsLog(fmt.Sprintf("Block found at seed %d! Submitting...", seed))

		result, err := rpcCall(nodeURL, "submitwork", []interface{}{
			address,
			seed,
			solutionStr,
			headerTimestamp,
		})
		if err != nil {
			jsLog(fmt.Sprintf("Submit error: %v", err))
			continue
		}

		var submitResult struct {
			Height    uint64 `json:"height"`
			BlockHash string `json:"block_hash"`
		}
		json.Unmarshal(result, &submitResult)
		jsLog(fmt.Sprintf("Block mined! Height: %d, Hash: %s", submitResult.Height, submitResult.BlockHash))

		mu.Lock()
		stats.BlocksMined++
		mu.Unlock()

		return nil
	}

	elapsed := time.Since(roundStart).Seconds()
	rate := float64(satSolved) / elapsed

	mu.Lock()
	stats.SATSolved += satSolved
	stats.SeedsTried += seedsPerRound
	stats.HashRate = rate
	stats.Elapsed = time.Since(globalStart).Seconds()
	mu.Unlock()

	jsLog(fmt.Sprintf("Round done: %d SAT solved in %.1fs (%.1f/s)", satSolved, elapsed, rate))
	return nil
}

func makeSATSeed(prevHash [32]byte, seed uint64) [32]byte {
	var buf [40]byte
	copy(buf[:32], prevHash[:])
	binary.LittleEndian.PutUint64(buf[32:], seed)
	return sha256.Sum256(buf[:])
}

func compactToTarget(bits uint32) *big.Int {
	exponent := bits >> 24
	mantissa := int64(bits & 0x007FFFFF)
	if bits&0x00800000 != 0 {
		mantissa = -mantissa
	}
	target := big.NewInt(mantissa)
	if exponent <= 3 {
		target.Rsh(target, uint(8*(3-exponent)))
	} else {
		target.Lsh(target, uint(8*(exponent-3)))
	}
	return target
}

func meetsTarget(hash [32]byte, target *big.Int) bool {
	hashInt := new(big.Int).SetBytes(hash[:])
	return hashInt.Cmp(target) <= 0
}

func doubleSha256(data []byte) [32]byte {
	first := sha256.Sum256(data)
	return sha256.Sum256(first[:])
}

func assignmentToString(a sat.Assignment) string {
	var b strings.Builder
	for _, v := range a {
		if v {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	return b.String()
}

type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func rpcCall(url, method string, params []interface{}) (json.RawMessage, error) {
	reqBody, _ := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	})

	rpcURL := url
	if !strings.HasSuffix(url, "/rpc") && !strings.HasSuffix(url, "/api") {
		rpcURL = url + "/rpc"
	}

	resp, err := http.Post(rpcURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var rpcResp rpcResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("RPC parse error: %s", string(body))
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}
