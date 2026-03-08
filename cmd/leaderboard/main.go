// leaderboard is a background service that scans the NOUS blockchain and
// writes a leaderboard.json file for the block explorer frontend.
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
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

type minerStat struct {
	Address    string `json:"address"`
	Blocks     int    `json:"blocks"`
	TotalNous  int    `json:"total_nous"`
	LastTs     int64  `json:"last_ts"`
	LastHeight int    `json:"last_height"`
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

// state kept in memory across update cycles
var (
	mu         sync.Mutex
	minerMap   = map[string]*minerStat{}
	diffPoints []diffPoint // all blocks, for chart
	lastScan   = -1        // last block height that was scanned
)

const batchSize = 50

func scanNewBlocks(height int) error {
	from := lastScan + 1
	if from > height {
		return nil // nothing new
	}
	log.Printf("leaderboard: scanning blocks %d..%d", from, height)

	for start := from; start <= height; start += batchSize {
		end := start + batchSize - 1
		if end > height {
			end = height
		}

		type blockResult struct {
			Height       int   `json:"height"`
			Timestamp    int64 `json:"timestamp"`
			Difficulty   int64 `json:"difficulty"`
			MinerAddress string `json:"miner_address"`
		}

		results := make([]blockResult, end-start+1)
		errs := make([]error, end-start+1)
		var wg sync.WaitGroup
		for h := start; h <= end; h++ {
			wg.Add(1)
			go func(h, i int) {
				defer wg.Done()
				raw, err := rpc("getblock", h)
				if err != nil {
					errs[i] = err
					return
				}
				json.Unmarshal(raw, &results[i])
			}(h, h-start)
		}
		wg.Wait()

		mu.Lock()
		for i, b := range results {
			if errs[i] != nil {
				continue
			}
			// difficulty chart data
			diffPoints = append(diffPoints, diffPoint{
				Height:     b.Height,
				Difficulty: b.Difficulty,
				Timestamp:  b.Timestamp,
			})
			if b.MinerAddress == "" {
				continue
			}
			s, ok := minerMap[b.MinerAddress]
			if !ok {
				s = &minerStat{Address: b.MinerAddress}
				minerMap[b.MinerAddress] = s
			}
			s.Blocks++
			s.TotalNous = s.Blocks
			if b.Timestamp > s.LastTs {
				s.LastTs = b.Timestamp
				s.LastHeight = b.Height
			}
		}
		mu.Unlock()
	}

	lastScan = height
	return nil
}

func buildJSON(height int) leaderboardData {
	mu.Lock()
	defer mu.Unlock()

	miners := make([]minerStat, 0, len(minerMap))
	for _, s := range minerMap {
		miners = append(miners, *s)
	}
	sort.Slice(miners, func(i, j int) bool {
		return miners[i].Blocks > miners[j].Blocks
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

	// Write difficulty chart JSON (last 200 points)
	mu.Lock()
	pts := diffPoints
	if len(pts) > 200 {
		pts = pts[len(pts)-200:]
	}
	diffData := diffChartData{UpdatedAt: time.Now().Unix(), Points: pts}
	mu.Unlock()

	diffFile := outFile[:len(outFile)-len("leaderboard.json")] + "difficulty.json"
	if err := writeJSON(diffFile, diffData); err != nil {
		log.Printf("leaderboard: write %s: %v", diffFile, err)
	}

	log.Printf("leaderboard: updated height=%d miners=%d", height, data.TotalMiners)
}

func main() {
	node := flag.String("node", "http://localhost:8332/rpc", "Node RPC URL")
	out  := flag.String("out", "/var/www/nous/leaderboard.json", "Output JSON file")
	interval := flag.Duration("interval", 30*time.Second, "Update interval")
	flag.Parse()

	nodeURL = *node

	log.Printf("leaderboard builder starting: node=%s out=%s interval=%s", nodeURL, *out, *interval)

	// Run immediately on start, then on interval
	update(*out)
	for range time.Tick(*interval) {
		update(*out)
	}
}
