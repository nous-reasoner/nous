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
)

func main() {
	reasoner := js.Global().Get("Object").New()
	reasoner.Set("solveBatch", js.FuncOf(solveBatch))
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

// --- solveBatch: pure computation, no RPC ---
// Args: workJSON string, seedStart int, seedEnd int
// Returns: JSON string {sat_solved, seeds_tried, found, seed, solution, header_timestamp, elapsed_ms}

type batchResult struct {
	SATSolved  int     `json:"sat_solved"`
	SeedsTried int     `json:"seeds_tried"`
	Found      bool    `json:"found"`
	Seed       uint64  `json:"seed"`
	Solution   string  `json:"solution"`
	HeaderTS   uint32  `json:"header_timestamp"`
	ElapsedMs  float64 `json:"elapsed_ms"`
	Error      string  `json:"error,omitempty"`
}

type workTemplate struct {
	Height    uint64 `json:"height"`
	PrevHash  string `json:"prev_hash"`
	DiffBits  uint32 `json:"difficulty_bits"`
	HeaderHex string `json:"header_hex"`
}

func solveBatch(this js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		r, _ := json.Marshal(batchResult{Error: "need workJSON, seedStart, seedEnd"})
		return string(r)
	}

	workJSON := args[0].String()
	seedStart := uint64(args[1].Float())
	seedEnd := uint64(args[2].Float())

	var work workTemplate
	if err := json.Unmarshal([]byte(workJSON), &work); err != nil {
		r, _ := json.Marshal(batchResult{Error: "parse work: " + err.Error()})
		return string(r)
	}

	headerTemplate, err := hex.DecodeString(work.HeaderHex)
	if err != nil || len(headerTemplate) != 148 {
		r, _ := json.Marshal(batchResult{Error: "invalid header template"})
		return string(r)
	}

	prevHashBytes, err := hex.DecodeString(work.PrevHash)
	if err != nil || len(prevHashBytes) != 32 {
		r, _ := json.Marshal(batchResult{Error: "invalid prev_hash"})
		return string(r)
	}
	var prevHash [32]byte
	copy(prevHash[:], prevHashBytes)

	target := compactToTarget(work.DiffBits)
	headerTimestamp := binary.LittleEndian.Uint32(headerTemplate[68:72])

	satSolved := 0
	start := time.Now()

	for seed := seedStart; seed < seedEnd; seed++ {
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
		if meetsTarget(blockHash, target) {
			solutionStr := assignmentToString(solution)
			r, _ := json.Marshal(batchResult{
				SATSolved:  satSolved,
				SeedsTried: int(seed - seedStart + 1),
				Found:      true,
				Seed:       seed,
				Solution:   solutionStr,
				HeaderTS:   headerTimestamp,
				ElapsedMs:  float64(time.Since(start).Milliseconds()),
			})
			return string(r)
		}
	}

	r, _ := json.Marshal(batchResult{
		SATSolved:  satSolved,
		SeedsTried: int(seedEnd - seedStart),
		Found:      false,
		ElapsedMs:  float64(time.Since(start).Milliseconds()),
	})
	return string(r)
}

// --- Wallet ---

func createWallet(this js.Value, args []js.Value) interface{} {
	var privKey [32]byte
	if _, err := rand.Read(privKey[:]); err != nil {
		return js.ValueOf(map[string]interface{}{"error": err.Error()})
	}

	sk := secp256k1.PrivKeyFromBytes(privKey[:])
	pubKey := sk.PubKey().SerializeCompressed()

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

// --- Helpers ---

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
