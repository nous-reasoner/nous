package integration

import (
	"math/big"
	"testing"
	"time"

	"github.com/nous-chain/nous/block"
	"github.com/nous-chain/nous/consensus"
	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/csp"
	"github.com/nous-chain/nous/network"
	"github.com/nous-chain/nous/node"
	"github.com/nous-chain/nous/storage"
	"github.com/nous-chain/nous/tx"
	"github.com/nous-chain/nous/wallet"
)

const NOU = int64(1_0000_0000)

// easyDifficulty returns params suitable for fast testing:
// minimal VDF iterations and the easiest possible PoW target.
func easyDifficulty() *consensus.DifficultyParams {
	maxTarget := new(big.Int).Lsh(big.NewInt(1), 256)
	maxTarget.Sub(maxTarget, big.NewInt(1))
	var target crypto.Hash
	b := maxTarget.Bytes()
	copy(target[32-len(b):], b)

	return &consensus.DifficultyParams{
		VDFIterations: 1,
		CSPDifficulty: consensus.CSPDifficultyParams{
			BaseVariables:   12,
			ConstraintRatio: 1.4,
		},
		PoWTarget: target,
	}
}

// ============================================================
// 1. TestFullMiningCycle
// ============================================================

func TestFullMiningCycle(t *testing.T) {
	// Generate miner key.
	privKey, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	minerPKH := crypto.Hash160(pubKey.SerializeCompressed())

	// Initialize chain with genesis block.
	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60)
	chain := consensus.NewChainState(genesis)
	chain.Difficulty = easyDifficulty()

	// Mine block 1.
	blk1, err := consensus.MineBlock(&genesis.Header, nil, privKey, pubKey, chain.Difficulty, 1, nil)
	if err != nil {
		t.Fatalf("mine block 1: %v", err)
	}
	if err := chain.AddBlock(blk1); err != nil {
		t.Fatalf("add block 1: %v", err)
	}

	// Verify miner balance = 10 NOUS after block 1.
	balance := chain.UTXOSet.GetBalance(minerPKH)
	if balance != 10*NOU {
		t.Fatalf("balance after block 1: want %d, got %d", 10*NOU, balance)
	}

	// Mine 5 more blocks (total height = 6).
	prev := &blk1.Header
	for h := uint64(2); h <= 6; h++ {
		blk, err := consensus.MineBlock(prev, nil, privKey, pubKey, chain.Difficulty, h, nil)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header
	}

	// Verify height = 6.
	if chain.Height != 6 {
		t.Fatalf("height: want 6, got %d", chain.Height)
	}

	// Verify miner balance = 60 NOUS (6 blocks × 10 NOUS).
	balance = chain.UTXOSet.GetBalance(minerPKH)
	if balance != 60*NOU {
		t.Fatalf("balance after 6 blocks: want %d, got %d", 60*NOU, balance)
	}
}

// ============================================================
// 2. TestTransferBetweenWallets
// ============================================================

func TestTransferBetweenWallets(t *testing.T) {
	// Create wallet A and wallet B.
	walletA, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	walletB, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	privA := walletA.Keys[walletA.Primary].PrivateKey
	pubA := walletA.Keys[walletA.Primary].PublicKey
	pkhA := walletA.PubKeyHash()
	pkhB := walletB.PubKeyHash()

	// Initialize chain with genesis (zero PKH).
	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60)
	chain := consensus.NewChainState(genesis)
	chain.Difficulty = easyDifficulty()

	// Mine block 1 with wallet A's key → A gets 10 NOUS.
	blk1, err := consensus.MineBlock(&genesis.Header, nil, privA, pubA, chain.Difficulty, 1, nil)
	if err != nil {
		t.Fatalf("mine block 1: %v", err)
	}
	if err := chain.AddBlock(blk1); err != nil {
		t.Fatalf("add block 1: %v", err)
	}

	// Mine 100 more blocks so block 1's coinbase matures (CoinbaseMaturity=100).
	prev := &blk1.Header
	for h := uint64(2); h <= 101; h++ {
		blk, err := consensus.MineBlock(prev, nil, privA, pubA, chain.Difficulty, h, nil)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header
	}

	balA := chain.UTXOSet.GetBalance(pkhA)
	// 101 blocks × 10 NOUS = 1010 NOUS (only first block's coinbase is mature enough to spend).
	if balA != 101*10*NOU {
		t.Fatalf("A balance after 101 blocks: want %d, got %d", 101*10*NOU, balA)
	}

	// Create transfer: A sends 5 NOUS to B with 0.001 NOUS fee.
	fee := int64(100_000) // 0.001 NOUS
	addrB := walletB.GetAddress()
	nextHeight := chain.Height + 1
	transfer, err := walletA.CreateTransaction(addrB, 5*NOU, fee, chain.UTXOSet, nextHeight)
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	// Mine block 102 with wallet A's key, including the transfer tx.
	blk102, err := consensus.MineBlock(prev, []*tx.Transaction{transfer}, privA, pubA, chain.Difficulty, nextHeight, nil, chain.UTXOSet)
	if err != nil {
		t.Fatalf("mine block 102: %v", err)
	}
	if err := chain.AddBlock(blk102); err != nil {
		t.Fatalf("add block 102: %v", err)
	}

	// Verify B balance = 5 NOUS.
	balB := chain.UTXOSet.GetBalance(pkhB)
	if balB != 5*NOU {
		t.Fatalf("B balance: want %d, got %d", 5*NOU, balB)
	}

	// Verify A balance = 1015 NOUS.
	// 102 blocks × 10 NOUS = 1020 - sent (5) = 1015.
	// Fee (0.001) paid by A is recovered as miner of block 102.
	expectedA := int64(1020-5) * NOU
	balA = chain.UTXOSet.GetBalance(pkhA)
	if balA != expectedA {
		t.Fatalf("A balance: want %d, got %d", expectedA, balA)
	}
}

// ============================================================
// 3. TestTwoNodeSync
// ============================================================

func TestTwoNodeSync(t *testing.T) {
	// Shared miner key.
	privKey, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	// Shared genesis so both nodes have the same chain root.
	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60)

	// --- Node A: mine 3 blocks ---
	chainA := consensus.NewChainState(genesis)
	chainA.Difficulty = easyDifficulty()

	storeA, err := storage.NewBlockStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := storeA.SaveBlock(genesis, 0); err != nil {
		t.Fatal(err)
	}

	cfgA := network.ServerConfig{ListenAddr: ":0", Magic: network.TestNetMagic}
	serverA := network.NewServer(cfgA)
	if err := serverA.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverA.Stop()

	minerA := node.NewMiner(chainA, serverA, storeA, privKey, pubKey)

	prev := &genesis.Header
	for h := uint64(1); h <= 3; h++ {
		blk, err := consensus.MineBlock(prev, nil, privKey, pubKey, chainA.Difficulty, h, nil)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := minerA.ApplyBlock(blk); err != nil {
			t.Fatalf("apply block %d to A: %v", h, err)
		}
		prev = &blk.Header
	}

	if chainA.Height != 3 {
		t.Fatalf("node A height: want 3, got %d", chainA.Height)
	}

	// --- Node B: start empty, sync blocks from A ---
	chainB := consensus.NewChainState(genesis)
	chainB.Difficulty = easyDifficulty()

	storeB, err := storage.NewBlockStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := storeB.SaveBlock(genesis, 0); err != nil {
		t.Fatal(err)
	}

	cfgB := network.ServerConfig{ListenAddr: ":0", Magic: network.TestNetMagic}
	serverB := network.NewServer(cfgB)
	if err := serverB.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverB.Stop()

	minerB := node.NewMiner(chainB, serverB, storeB, privKey, pubKey)

	// Transfer all 3 blocks from A to B.
	for h := uint64(1); h <= 3; h++ {
		blk, err := storeA.LoadBlockByHeight(h)
		if err != nil {
			t.Fatalf("load block %d from A: %v", h, err)
		}
		if err := minerB.ApplyBlock(blk); err != nil {
			t.Fatalf("apply block %d to B: %v", h, err)
		}
	}

	// Verify B synced to height 3.
	if chainB.Height != 3 {
		t.Fatalf("node B height: want 3, got %d", chainB.Height)
	}

	// Verify chain tip hashes match.
	tipA := chainA.Tip.Hash()
	tipB := chainB.Tip.Hash()
	if tipA != tipB {
		t.Fatalf("tip hash mismatch: A=%x B=%x", tipA[:8], tipB[:8])
	}
}

// ============================================================
// 4. TestMiningWithCSPSolver
// ============================================================

func TestMiningWithCSPSolver(t *testing.T) {
	privKey, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60)
	chain := consensus.NewChainState(genesis)
	chain.Difficulty = easyDifficulty()

	// Mine a single block.
	blk, err := consensus.MineBlock(&genesis.Header, nil, privKey, pubKey, chain.Difficulty, 1, nil)
	if err != nil {
		t.Fatalf("mine block: %v", err)
	}
	if err := chain.AddBlock(blk); err != nil {
		t.Fatalf("add block: %v", err)
	}

	// Block must contain a valid standard CSP solution.
	if blk.CSPSolution == nil {
		t.Fatal("block has no CSP solution")
	}

	// Regenerate the CSP problem from the same VDF output and verify.
	seed := crypto.Sha256(blk.Header.VDFOutput)
	problem, _ := csp.GenerateProblem(seed, csp.Standard)

	if !csp.VerifySolution(problem, blk.CSPSolution) {
		t.Fatal("CSP solution does not verify against regenerated problem")
	}

	// Verify solution hash in header matches.
	solHash := consensus.HashSolutionValues(blk.CSPSolution.Values)
	if solHash != blk.Header.CSPSolutionHash {
		t.Fatalf("CSP solution hash mismatch: header=%x computed=%x",
			blk.Header.CSPSolutionHash[:8], solHash[:8])
	}
}
