package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	NodeURL    string
	AIProvider string
	APIKey     string
	Model      string
	Address    string
	BaseURL    string
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
	flag.StringVar(&config.AIProvider, "ai", "openai", "AI provider")
	flag.StringVar(&config.APIKey, "key", "", "API key")
	flag.StringVar(&config.Model, "model", "gpt-4o", "Model name")
	flag.StringVar(&config.Address, "address", "", "Mining address")
	flag.StringVar(&config.BaseURL, "base-url", "", "Custom API base URL")
	flag.Parse()

	if config.Address == "" {
		log.Fatal("Mining address required")
	}

	log.Printf("Starting miner: node=%s, ai=%s, model=%s", config.NodeURL, config.AIProvider, config.Model)

	for {
		if err := mineBlock(config); err != nil {
			log.Printf("Mining error: %v", err)
			time.Sleep(10 * time.Second)
		}
	}
}

func mineBlock(config Config) error {
	// Try seeds 0..99 per round.
	for seed := uint64(0); seed < 100; seed++ {
		// Get work from node (includes actual SAT formula).
		workRaw, err := rpcCall(config.NodeURL, "getwork", []interface{}{seed})
		if err != nil {
			return fmt.Errorf("getwork: %w", err)
		}

		var work struct {
			Height        uint64 `json:"height"`
			PrevHash      string `json:"prev_hash"`
			DiffBits      uint32 `json:"difficulty_bits"`
			Seed          uint64 `json:"seed"`
			NVars         int    `json:"n_vars"`
			NClauses      int    `json:"n_clauses"`
			Formula       string `json:"formula"`
		}
		if err := json.Unmarshal(workRaw, &work); err != nil {
			return fmt.Errorf("parse work: %w", err)
		}

		if seed == 0 {
			log.Printf("Mining block %d (difficulty: 0x%08x)", work.Height, work.DiffBits)
		}

		// Ask AI to solve the actual SAT formula.
		prompt := fmt.Sprintf(`Solve this 3-SAT problem. Find a variable assignment that satisfies ALL clauses.

%s
Return ONLY a %d-character binary string (0s and 1s) where position i is the value of variable i+1.
No explanation, no formatting, just the binary string.`, work.Formula, work.NVars)

		response, err := callAI(config, prompt)
		if err != nil {
			return fmt.Errorf("AI (seed=%d): %w", seed, err)
		}

		// Extract the 256-bit binary string from AI response.
		solution := extractBinaryString(response, work.NVars)
		if solution == "" {
			log.Printf("Seed %d: AI did not return valid %d-bit binary string, trying next seed", seed, work.NVars)
			continue
		}

		log.Printf("Seed %d: solution=%s...", seed, solution[:32])

		// Submit work to node.
		result, err := rpcCall(config.NodeURL, "submitwork", []interface{}{
			config.Address,
			seed,
			solution,
		})
		if err != nil {
			if strings.Contains(err.Error(), "does not meet difficulty") ||
				strings.Contains(err.Error(), "does not satisfy") {
				log.Printf("Seed %d: %v, trying next seed", seed, err)
				continue
			}
			return fmt.Errorf("submitwork: %w", err)
		}

		var submitResult struct {
			Height    uint64 `json:"height"`
			BlockHash string `json:"block_hash"`
		}
		json.Unmarshal(result, &submitResult)
		log.Printf("Block found! Height: %d, Hash: %s", submitResult.Height, submitResult.BlockHash)
		return nil
	}

	log.Printf("No valid block found in 100 seeds, retrying...")
	return nil
}

// extractBinaryString finds the first N-length binary string in the AI response.
func extractBinaryString(text string, n int) string {
	// Try to find an exact N-length binary string.
	pattern := fmt.Sprintf(`[01]{%d}`, n)
	re := regexp.MustCompile(pattern)
	match := re.FindString(text)
	if match != "" {
		return match
	}

	// Fallback: collect all 0s and 1s from the text and take the first N.
	var bits strings.Builder
	for _, ch := range text {
		if ch == '0' || ch == '1' {
			bits.WriteRune(ch)
			if bits.Len() >= n {
				return bits.String()[:n]
			}
		}
	}
	return ""
}

func callAI(config Config, prompt string) (string, error) {
	switch config.AIProvider {
	case "openai":
		return callOpenAI(config, prompt)
	case "anthropic":
		return callAnthropic(config, prompt)
	case "ollama":
		return callOllama(config, prompt)
	default:
		return "", fmt.Errorf("unknown AI provider: %s", config.AIProvider)
	}
}

func callOpenAI(config Config, prompt string) (string, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": config.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.9,
	})

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	req, _ := http.NewRequest("POST", baseURL+"/v1/chat/completions", bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "Bearer "+config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	return result.Choices[0].Message.Content, nil
}

func callAnthropic(config Config, prompt string) (string, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": config.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 1024,
	})

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	req, _ := http.NewRequest("POST", baseURL+"/v1/messages", bytes.NewBuffer(reqBody))
	req.Header.Set("x-api-key", config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no response from Anthropic")
	}

	return result.Content[0].Text, nil
}

func callOllama(config Config, prompt string) (string, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":  config.Model,
		"prompt": prompt,
		"stream": false,
	})

	resp, err := http.Post(baseURL+"/api/generate", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Response, nil
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

	resp, err := http.Post(rpcURL, "application/json", bytes.NewBuffer(reqBody))
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
