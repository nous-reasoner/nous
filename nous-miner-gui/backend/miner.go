package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/nous-reasoner/nous-miner-gui/backend/sat"
	"github.com/nous-reasoner/nous-miner-gui/backend/solver"
)

type Config struct {
	NodeURL    string
	Address    string
	SolverName string
	// AI config (for ai-guided and pure-ai solvers)
	AIProvider string
	APIKey     string
	Model      string
	BaseURL    string
	// Custom solver
	ScriptPath string
}

type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type RPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	config := Config{}
	flag.StringVar(&config.NodeURL, "node", "http://localhost:8332", "Node RPC URL")
	flag.StringVar(&config.Address, "address", "", "Mining address")
	flag.StringVar(&config.SolverName, "solver", "probsat", "Solver: probsat, ai-guided, pure-ai, custom")
	flag.StringVar(&config.AIProvider, "ai-provider", "anthropic", "AI provider: openai, anthropic")
	flag.StringVar(&config.APIKey, "api-key", "", "AI API key")
	flag.StringVar(&config.Model, "model", "claude-sonnet-4-6", "AI model name")
	flag.StringVar(&config.BaseURL, "base-url", "", "Custom API base URL")
	flag.StringVar(&config.ScriptPath, "script", "", "Custom solver script path")
	flag.Parse()

	if config.Address == "" {
		log.Fatal("Mining address required")
	}

	// Configure solver.
	solv, err := solver.Get(config.SolverName)
	if err != nil {
		log.Fatal(err)
	}
	configureSolver(solv, config)

	log.Printf("Starting miner: node=%s, solver=%s", config.NodeURL, solv.Name())

	for {
		if err := mineBlock(config, solv); err != nil {
			log.Printf("Mining error: %v", err)
			time.Sleep(10 * time.Second)
		}
	}
}

func configureSolver(s solver.Solver, config Config) {
	switch v := s.(type) {
	case *solver.AIGuidedSolver:
		v.Provider = config.AIProvider
		v.APIKey = config.APIKey
		v.Model = config.Model
		v.BaseURL = config.BaseURL
		v.UseFallback = true
	case *solver.PureAISolver:
		v.Provider = config.AIProvider
		v.APIKey = config.APIKey
		v.Model = config.Model
		v.BaseURL = config.BaseURL
	case *solver.CustomSolver:
		v.ScriptPath = config.ScriptPath
	}
}

func mineBlock(config Config, solv solver.Solver) error {
	// Single RPC call to get header template and mining info.
	workRaw, err := rpcCall(config.NodeURL, "getwork", []interface{}{config.Address, uint64(0)})
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

	log.Printf("Mining block %d (difficulty: 0x%08x) with %s", work.Height, work.DiffBits, solv.Name())

	// Reset AI bias for new round (new block height = new formulas).
	if rr, ok := solv.(interface{ ResetBias() }); ok {
		rr.ResetBias()
	}

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
	// Extract timestamp from header template (offset 68, uint32 LE).
	headerTimestamp := binary.LittleEndian.Uint32(headerTemplate[68:72])
	satSolved := 0
	startTime := time.Now()

	for seed := uint64(0); seed < 10000; seed++ {
		// Generate SAT formula locally.
		satSeed := sat.MakeSATSeed(prevHash, seed)
		formula := sat.GenerateFormula(satSeed, sat.SATVariables, sat.SATClausesRatio)

		// Solve with configured solver.
		solution, err := solv.Solve(formula, sat.SATVariables)
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
				elapsed := time.Since(startTime).Seconds()
				rate := float64(satSolved) / elapsed
				log.Printf("Progress: %d SAT solved (%.1f/s), seed=%d, no PoW match yet", satSolved, rate, seed)
			}
			continue
		}

		// PoW met — submit.
		solutionStr := assignmentToString(solution)
		log.Printf("Seed %d: SAT solved + PoW met! Submitting...", seed)

		result, err := rpcCall(config.NodeURL, "submitwork", []interface{}{
			config.Address,
			seed,
			solutionStr,
			headerTimestamp,
		})
		if err != nil {
			log.Printf("Seed %d: submit error: %v", seed, err)
			continue
		}

		var submitResult struct {
			Height    uint64 `json:"height"`
			BlockHash string `json:"block_hash"`
		}
		json.Unmarshal(result, &submitResult)
		log.Printf("Block found! Height: %d, Hash: %s", submitResult.Height, submitResult.BlockHash)
		return nil
	}

	elapsed := time.Since(startTime).Seconds()
	log.Printf("Round: %d SAT solved in %.1fs (%.1f/s), no block", satSolved, elapsed, float64(satSolved)/elapsed)
	return nil
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

func rpcCall(url, method string, params []interface{}) (json.RawMessage, error) {
	reqBody, _ := json.Marshal(RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	})

	rpcURL := url
	if !strings.HasSuffix(url, "/rpc") && !strings.HasSuffix(url, "/api") && !strings.Contains(url, "/api/") {
		rpcURL = url + "/rpc"
	}

	resp, err := http.Post(rpcURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var rpcResp RPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("RPC parse error: %s", string(body))
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}
