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
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"nous/consensus"
	"nous/sat"
)

type RPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

type RPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

var rpcClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        5,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
	},
}

func main() {
	nodeURL := flag.String("node", "http://localhost:8332", "Node RPC URL")
	address := flag.String("address", "", "Reasoning reward address (nous1q...)")
	threads := flag.Int("threads", runtime.NumCPU(), "Number of solver threads")
	seedsPerRound := flag.Uint64("seeds", 10000, "Seeds to try per round")
	flag.Parse()

	if *address == "" {
		log.Fatal("Address required: --address nous1q...")
	}

	log.Printf("NOUS Reasoner: node=%s, address=%s, threads=%d", *nodeURL, *address, *threads)

	retryDelay := time.Second
	for {
		if err := mineRound(*nodeURL, *address, *threads, *seedsPerRound); err != nil {
			log.Printf("Error: %v", err)
			time.Sleep(retryDelay)
			if retryDelay < 10*time.Second {
				retryDelay *= 2
			}
		} else {
			retryDelay = time.Second
		}
	}
}

func mineRound(nodeURL, address string, numThreads int, maxSeeds uint64) error {
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

	log.Printf("Reasoning block %d (difficulty: 0x%08x)", work.Height, work.DiffBits)

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

	startTime := time.Now()
	var totalSolved atomic.Int64
	var seedCounter atomic.Uint64

	type blockResult struct {
		seed      uint64
		solution  string
		height    uint64
		blockHash string
	}
	found := make(chan blockResult, 1)
	done := make(chan struct{})

	for t := 0; t < numThreads; t++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
				}

				seed := seedCounter.Add(1) - 1
				if seed >= maxSeeds {
					return
				}

				satSeed := consensus.MakeSATSeed(prevHash, seed)
				formula := sat.GenerateFormula(satSeed, consensus.SATVariables, consensus.SATClausesRatio)

				solution, err := sat.ProbSATSolve(formula, consensus.SATVariables, 100*time.Millisecond)
				if err != nil {
					continue
				}

				if !sat.Verify(formula, solution) {
					continue
				}
				totalSolved.Add(1)
				solved := totalSolved.Load()

				solBytes := sat.SerializeAssignment(solution)
				solHash := sha256.Sum256(solBytes)

				header := make([]byte, 148)
				copy(header, headerTemplate)
				binary.LittleEndian.PutUint64(header[76:84], seed)
				copy(header[84:116], solHash[:])

				blockHash := doubleSha256(header)
				if !meetsTarget(blockHash, target) {
					if solved%50 == 0 {
						elapsed := time.Since(startTime).Seconds()
						rate := float64(solved) / elapsed
						log.Printf("Progress: %d SAT solved (%.1f/s), seed=%d", solved, rate, seed)
					}
					continue
				}

				solutionStr := assignmentToString(solution)
				log.Printf("Seed %d: SAT solved + PoW met! Submitting...", seed)

				result, err := rpcCall(nodeURL, "submitwork", []interface{}{
					address,
					seed,
					solutionStr,
					headerTimestamp,
				})
				if err != nil {
					log.Printf("Seed %d: submit error: %v", seed, err)
					continue
				}

				var submitRes struct {
					Height    uint64 `json:"height"`
					BlockHash string `json:"block_hash"`
				}
				json.Unmarshal(result, &submitRes)

				select {
				case found <- blockResult{seed, solutionStr, submitRes.Height, submitRes.BlockHash}:
				default:
				}
				return
			}
		}()
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case res := <-found:
			close(done)
			solved := totalSolved.Load()
			elapsed := time.Since(startTime).Seconds()
			rate := float64(solved) / elapsed
			log.Printf("Block found! Height: %d, Hash: %s (%.1fs, %d SAT solved, %.1f/s)",
				res.height, res.blockHash, elapsed, solved, rate)
			return nil
		case <-ticker.C:
			if seedCounter.Load() >= maxSeeds {
				close(done)
				solved := totalSolved.Load()
				elapsed := time.Since(startTime).Seconds()
				log.Printf("Round: %d SAT solved in %.1fs (%.1f/s), no block",
					solved, elapsed, float64(solved)/elapsed)
				return nil
			}
		}
	}
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

func rpcCall(url, method string, params interface{}) (json.RawMessage, error) {
	reqBody, _ := json.Marshal(RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	})

	rpcURL := url
	if !strings.HasSuffix(url, "/rpc") && !strings.HasSuffix(url, "/api") {
		rpcURL = url + "/rpc"
	}

	resp, err := rpcClient.Post(rpcURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
	}

	var rpcResp RPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("RPC parse error: %s", snippet)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}
