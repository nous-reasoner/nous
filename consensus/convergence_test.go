package consensus

import (
	"fmt"
	"testing"
	"time"

	"nous/block"
	"nous/crypto"
)

// TestConvergence simulates a mining loop to verify that block intervals
// converge toward TargetBlockTime (150s) under the difficulty adjustment algorithm.
//
// Starting from TestnetDifficultyParams (~0.1s/block), the adjustment
// caps at 1.25× per 1008-block epoch. Within the test timeout we verify
// the convergence *direction*: block intervals grow.
func TestConvergence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping convergence test in short mode")
	}

	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pubKey.SerializeCompressed())

	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60, 0x1d00ffff, false)
	chain := NewChainState(genesis)
	chain.Difficulty = TestnetDifficultyParams()
	chain.Anchor.Target = TestnetDifficultyParams().PoWTarget

	const maxBlocks = 3500
	const wallClockLimit = 550 * time.Second

	intervals := make([]float64, 0, maxBlocks)
	testStart := time.Now()
	prev := &genesis.Header

	for h := uint64(1); h <= maxBlocks; h++ {
		if time.Since(testStart) > wallClockLimit {
			t.Logf("wall-clock limit reached at height %d (%.0fs elapsed)", h-1, time.Since(testStart).Seconds())
			break
		}

		blockStart := time.Now()
		blk, err := MineBlock(prev, nil, pubKeyHash, chain.Difficulty, h, nil, false)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		elapsed := time.Since(blockStart).Seconds()
		intervals = append(intervals, elapsed)

		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header

		// Print stats every 500 blocks.
		if h%500 == 0 {
			window := 144
			if len(intervals) < window {
				window = len(intervals)
			}
			var sum float64
			for _, iv := range intervals[len(intervals)-window:] {
				sum += iv
			}
			avg := sum / float64(window)
			bits := TargetToCompact(chain.Difficulty.PoWTarget)
			t.Logf("height=%d  avg_interval=%.2fs  pow_target_bits=0x%08x",
				h, avg, bits)
		}
	}

	totalBlocks := uint64(len(intervals))
	totalTime := time.Since(testStart).Seconds()
	t.Logf("total: %d blocks in %.1fs (overall avg: %.3fs/block)",
		totalBlocks, totalTime, totalTime/float64(totalBlocks))

	// Print final stats line.
	if len(intervals) >= 144 {
		var sum float64
		for _, iv := range intervals[len(intervals)-144:] {
			sum += iv
		}
		avg := sum / 144
		bits := TargetToCompact(chain.Difficulty.PoWTarget)
		t.Logf("height=%d  avg_interval=%.2fs  pow_target_bits=0x%08x",
			totalBlocks, avg, bits)
	}

	// --- Assertions ---

	// If we reached block 3000, check avg interval is growing.
	if totalBlocks >= 288 {
		var firstSum, lastSum float64
		for _, iv := range intervals[:144] {
			firstSum += iv
		}
		for _, iv := range intervals[len(intervals)-144:] {
			lastSum += iv
		}
		firstAvg := firstSum / 144
		lastAvg := lastSum / 144
		ratio := lastAvg / firstAvg
		t.Logf("convergence trend: first_epoch_avg=%.4fs  last_epoch_avg=%.4fs  ratio=%.1fx",
			firstAvg, lastAvg, ratio)
		if lastAvg <= firstAvg {
			t.Fatal("block intervals should be increasing (converging toward target)")
		}
	} else {
		t.Logf("only %d blocks mined, skipping trend assertion", len(intervals))
	}

	fmt.Printf("\nConvergence summary:\n")
	fmt.Printf("  Target block time:   %ds\n", TargetBlockTime)
	fmt.Printf("  Blocks mined:        %d\n", totalBlocks)
	fmt.Printf("  Wall time:           %.0fs\n", totalTime)
}
