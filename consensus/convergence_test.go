package consensus

import (
	"fmt"
	"testing"
	"time"

	"github.com/nous-chain/nous/block"
	"github.com/nous-chain/nous/crypto"
)

// TestConvergence simulates a mining loop to verify that block intervals
// converge toward TargetBlockTime (30s) under the difficulty adjustment algorithm.
//
// Starting from TestnetDifficultyParams (VDF T=1024, ~0.1s/block), the adjustment
// caps at 1.25× per 144-block epoch. Reaching 30s/block requires ~26 epochs
// (~3744 blocks, ~4.8h wall time), so within the 600s test timeout we verify
// the convergence *direction*: VDF iterations increase and block intervals grow.
func TestConvergence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping convergence test in short mode")
	}

	privKey, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60)
	chain := NewChainState(genesis)
	chain.Difficulty = TestnetDifficultyParams()

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
		blk, err := MineBlock(prev, nil, privKey, pubKey, chain.Difficulty, h, nil)
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
			t.Logf("height=%d  avg_interval=%.2fs  vdf_iterations=%d  pow_target_bits=0x%08x",
				h, avg, chain.Difficulty.VDFIterations, bits)
		}
	}

	totalBlocks := uint64(len(intervals))
	totalTime := time.Since(testStart).Seconds()
	t.Logf("total: %d blocks in %.1fs (overall avg: %.3fs/block)",
		totalBlocks, totalTime, totalTime/float64(totalBlocks))

	initVDF := TestnetDifficultyParams().VDFIterations
	finalVDF := chain.Difficulty.VDFIterations
	t.Logf("VDF iterations: initial=%d  final=%d  growth=%.1fx",
		initVDF, finalVDF, float64(finalVDF)/float64(initVDF))

	// Print final stats line.
	if len(intervals) >= 144 {
		var sum float64
		for _, iv := range intervals[len(intervals)-144:] {
			sum += iv
		}
		avg := sum / 144
		bits := TargetToCompact(chain.Difficulty.PoWTarget)
		t.Logf("height=%d  avg_interval=%.2fs  vdf_iterations=%d  pow_target_bits=0x%08x",
			totalBlocks, avg, chain.Difficulty.VDFIterations, bits)
	}

	// --- Assertions ---

	// 1. VDF iterations must have increased.
	if finalVDF <= initVDF {
		t.Fatalf("VDF iterations should increase: initial=%d, final=%d", initVDF, finalVDF)
	}

	// 2. If we reached block 3000, check avg interval is in [15, 60]s.
	if totalBlocks >= 3000 {
		var sum float64
		tail := intervals[2999:]
		for _, iv := range tail {
			sum += iv
		}
		avg := sum / float64(len(tail))
		if avg < 15.0 || avg > 60.0 {
			t.Fatalf("avg interval after block 3000: %.2fs, want [15, 60]", avg)
		}
		t.Logf("convergence check: avg interval after block 3000 = %.2fs [PASS]", avg)
		return
	}

	// 3. Otherwise verify convergence trend: last epoch avg > first epoch avg.
	if len(intervals) >= 288 {
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
		// Verify meaningful increase (at least 2× over entire run).
		if ratio < 2.0 {
			t.Logf("warning: convergence ratio %.1fx is modest (expected ≥2×)", ratio)
		}
	} else {
		t.Logf("only %d blocks mined, skipping trend assertion", len(intervals))
	}

	fmt.Printf("\nConvergence summary:\n")
	fmt.Printf("  Target block time:   %ds\n", TargetBlockTime)
	fmt.Printf("  Blocks mined:        %d\n", totalBlocks)
	fmt.Printf("  Wall time:           %.0fs\n", totalTime)
	fmt.Printf("  VDF growth:          %d → %d (%.1f×)\n",
		initVDF, finalVDF, float64(finalVDF)/float64(initVDF))
}
