//go:build js && wasm

package main

import (
	"crypto/rand"
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

	"nous/crypto"
	"nous/sat"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

const (
	satVariables    = 256
	satClausesRatio = 3.85
	satSolveTimeout = 100 * time.Millisecond
	seedsPerRound   = 10000
)

var (
	reasoning bool
	stopCh    chan struct{}
	mu        sync.Mutex
	stats     reasoningStats
)

type reasoningStats struct {
	SATSolved    int     `json:"sat_solved"`
	SeedsTried   int     `json:"seeds_tried"`
	BlocksFound  int     `json:"blocks_found"`
	SolveRate    float64 `json:"solve_rate"`
	Running      bool    `json:"running"`
	Height       uint64  `json:"height"`
	Elapsed      float64 `json:"elapsed"`
}

func main() {
	reasoner := js.Global().Get("Object").New()
	reasoner.Set("start", js.FuncOf(startReasoning))
	reasoner.Set("stop", js.FuncOf(stopReasoning))
	reasoner.Set("getStats", js.FuncOf(getStats))
	reasoner.Set("createWallet", js.FuncOf(createWallet))
	reasoner.Set("getBalance", js.FuncOf(getBalance))
	js.Global().Set("nousReasoner", reasoner)

	jsLog("NOUS Reasoner loaded")

	select {}
}

func jsLog(msg string) {
	cb := js.Global().Get("postReasonerLog")
	if !cb.IsUndefined() && !cb.IsNull() {
		cb.Invoke(msg)
	}
}

// --- Wallet ---

func createWallet(this js.Value, args []js.Value) interface{} {
	// Generate 32-byte private key.
	var privKey [32]byte
	if _, err := rand.Read(privKey[:]); err != nil {
		return js.ValueOf(map[string]interface{}{"error": err.Error()})
	}

	// Derive compressed public key.
	sk := secp256k1.PrivKeyFromBytes(privKey[:])
	pubKey := sk.PubKey().SerializeCompressed()

	// hash160 → bech32m address.
	pkh := crypto.Hash160(pubKey)
	address := pubKeyHashToAddress(pkh)

	result := js.Global().Get("Object").New()
	result.Set("private_key", hex.EncodeToString(privKey[:]))
	result.Set("public_key", hex.EncodeToString(pubKey))
	result.Set("address", address)
	return result
}

func pubKeyHashToAddress(pkh []byte) string {
	data5 := convertBits(pkh, 8, 5, true)
	return bech32mEncode("nous", append([]int{0}, data5...))
}

// --- Bech32m ---

const bech32mConst = 0x2bc830a3
const charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

func bech32mPolymod(values []int) int {
	gen := [5]int{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := 1
	for _, v := range values {
		b := chk >> 25
		chk = ((chk & 0x1ffffff) << 5) ^ v
		for i := 0; i < 5; i++ {
			if (b>>i)&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

func bech32mHrpExpand(hrp string) []int {
	ret := make([]int, 0, len(hrp)*2+1)
	for _, c := range hrp {
		ret = append(ret, int(c)>>5)
	}
	ret = append(ret, 0)
	for _, c := range hrp {
		ret = append(ret, int(c)&31)
	}
	return ret
}

func bech32mEncode(hrp string, data5 []int) string {
	values := append(bech32mHrpExpand(hrp), data5...)
	polymod := bech32mPolymod(append(values, 0, 0, 0, 0, 0, 0)) ^ bech32mConst
	checksum := make([]int, 6)
	for i := 0; i < 6; i++ {
		checksum[i] = (polymod >> (5 * (5 - i))) & 31
	}
	var b strings.Builder
	b.WriteString(hrp)
	b.WriteByte('1')
	for _, d := range append(data5, checksum...) {
		b.WriteByte(charset[d])
	}
	return b.String()
}

func convertBits(data []byte, fromBits, toBits int, pad bool) []int {
	acc, bits := 0, 0
	var ret []int
	maxv := (1 << toBits) - 1
	for _, d := range data {
		acc = (acc << fromBits) | int(d)
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			ret = append(ret, (acc>>bits)&maxv)
		}
	}
	if pad && bits > 0 {
		ret = append(ret, (acc<<(toBits-bits))&maxv)
	}
	return ret
}

// --- Balance ---

func getBalance(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return js.Null()
	}
	nodeURL := args[0].String()
	address := args[1].String()

	promise := js.Global().Get("Promise").New(js.FuncOf(func(_ js.Value, promArgs []js.Value) interface{} {
		resolve := promArgs[0]
		reject := promArgs[1]
		go func() {
			result, err := rpcCall(nodeURL, "getbalance", []interface{}{address})
			if err != nil {
				reject.Invoke(err.Error())
				return
			}
			var bal struct {
				Balance  int64 `json:"balance"`
				Immature int64 `json:"immature"`
			}
			json.Unmarshal(result, &bal)
			obj := js.Global().Get("Object").New()
			obj.Set("balance", bal.Balance)
			obj.Set("immature", bal.Immature)
			resolve.Invoke(obj)
		}()
		return nil
	}))
	return promise
}

// --- Reasoning ---

func startReasoning(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return "error: need nodeURL and address"
	}
	nodeURL := args[0].String()
	address := args[1].String()

	mu.Lock()
	if reasoning {
		mu.Unlock()
		return "already reasoning"
	}
	reasoning = true
	stopCh = make(chan struct{})
	stats = reasoningStats{Running: true}
	mu.Unlock()

	go reasonLoop(nodeURL, address)
	return "started"
}

func stopReasoning(this js.Value, args []js.Value) interface{} {
	mu.Lock()
	defer mu.Unlock()
	if !reasoning {
		return "not reasoning"
	}
	close(stopCh)
	reasoning = false
	stats.Running = false
	jsLog("Reasoning stopped")
	return "stopped"
}

func getStats(this js.Value, args []js.Value) interface{} {
	mu.Lock()
	defer mu.Unlock()
	b, _ := json.Marshal(stats)
	return string(b)
}

func reasonLoop(nodeURL, address string) {
	defer func() {
		mu.Lock()
		reasoning = false
		stats.Running = false
		mu.Unlock()
	}()

	jsLog(fmt.Sprintf("Reasoning started: node=%s", nodeURL))
	globalStart := time.Now()

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		err := reasonRound(nodeURL, address, globalStart)
		if err != nil {
			jsLog(fmt.Sprintf("Error: %v", err))
			select {
			case <-stopCh:
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func reasonRound(nodeURL, address string, globalStart time.Time) error {
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

	jsLog(fmt.Sprintf("Reasoning block %d (diff: 0x%08x)", work.Height, work.DiffBits))

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

		solBytes := sat.SerializeAssignment(solution)
		solHash := sha256.Sum256(solBytes)

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
				stats.SolveRate = rate
				stats.Elapsed = time.Since(globalStart).Seconds()
				mu.Unlock()
				jsLog(fmt.Sprintf("Progress: %d SAT solved (%.1f/s), seed=%d", satSolved, rate, seed))
			}
			continue
		}

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
		jsLog(fmt.Sprintf("Block reasoned! Height: %d, Hash: %s", submitResult.Height, submitResult.BlockHash))

		mu.Lock()
		stats.BlocksFound++
		mu.Unlock()

		return nil
	}

	elapsed := time.Since(roundStart).Seconds()
	rate := float64(satSolved) / elapsed

	mu.Lock()
	stats.SATSolved += satSolved
	stats.SeedsTried += seedsPerRound
	stats.SolveRate = rate
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
