package integration

import (
	"math/big"
	"sync"
	"testing"
	"time"

	"nous/block"
	"nous/consensus"
	"nous/crypto"
	"nous/network"
	"nous/node"
	"nous/storage"
	"nous/tx"
	"nous/wallet"
)

const NOU = int64(1_0000_0000)

// easyDifficulty returns params suitable for fast testing.
func easyDifficulty() *consensus.DifficultyParams {
	maxTarget := new(big.Int).Lsh(big.NewInt(1), 256)
	maxTarget.Sub(maxTarget, big.NewInt(1))
	var target crypto.Hash
	b := maxTarget.Bytes()
	copy(target[32-len(b):], b)

	return &consensus.DifficultyParams{
		PoWTarget: target,
	}
}

// ============================================================
// 1. TestFullMiningCycle
// ============================================================

func TestFullMiningCycle(t *testing.T) {
	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	minerPKH := crypto.Hash160(pubKey.SerializeCompressed())

	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60, 0x1d00ffff, false)
	chain := consensus.NewChainState(genesis)
	chain.Difficulty = easyDifficulty()
	chain.Anchor.Target = easyDifficulty().PoWTarget

	// Mine block 1.
	blk1, err := consensus.MineBlock(&genesis.Header, nil, minerPKH, chain.Difficulty, 1, nil, false)
	if err != nil {
		t.Fatalf("mine block 1: %v", err)
	}
	if err := chain.AddBlock(blk1); err != nil {
		t.Fatalf("add block 1: %v", err)
	}

	// Verify miner balance = 1 NOUS after block 1.
	balance := chain.UTXOSet.GetBalance(minerPKH)
	if balance != 1*NOU {
		t.Fatalf("balance after block 1: want %d, got %d", 1*NOU, balance)
	}

	// Mine 5 more blocks (total height = 6).
	prev := &blk1.Header
	for h := uint64(2); h <= 6; h++ {
		blk, err := consensus.MineBlock(prev, nil, minerPKH, chain.Difficulty, h, nil, false)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header
	}

	if chain.Height != 6 {
		t.Fatalf("height: want 6, got %d", chain.Height)
	}

	// Verify miner balance = 6 NOUS (6 blocks × 1 NOUS).
	balance = chain.UTXOSet.GetBalance(minerPKH)
	if balance != 6*NOU {
		t.Fatalf("balance after 6 blocks: want %d, got %d", 6*NOU, balance)
	}
}

// ============================================================
// 2. TestTransferBetweenWallets
// ============================================================

func TestTransferBetweenWallets(t *testing.T) {
	walletA, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	walletB, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	pubA := walletA.Keys[walletA.Primary].PublicKey
	pkhA := walletA.PubKeyHash()
	pkhB := walletB.PubKeyHash()

	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60, 0x1d00ffff, false)
	chain := consensus.NewChainState(genesis)
	chain.Difficulty = easyDifficulty()
	chain.Anchor.Target = easyDifficulty().PoWTarget

	// Mine block 1 with wallet A's key → A gets 1 NOUS.
	blk1, err := consensus.MineBlock(&genesis.Header, nil, pkhA, chain.Difficulty, 1, nil, false)
	if err != nil {
		t.Fatalf("mine block 1: %v", err)
	}
	if err := chain.AddBlock(blk1); err != nil {
		t.Fatalf("add block 1: %v", err)
	}

	// Mine 100 more blocks so block 1's coinbase matures (CoinbaseMaturity=100).
	prev := &blk1.Header
	for h := uint64(2); h <= 101; h++ {
		blk, err := consensus.MineBlock(prev, nil, pkhA, chain.Difficulty, h, nil, false)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header
	}

	balA := chain.UTXOSet.GetBalance(pkhA)
	// 101 blocks × 1 NOUS = 101 NOUS.
	if balA != 101*NOU {
		t.Fatalf("A balance after 101 blocks: want %d, got %d", 101*NOU, balA)
	}

	// Create transfer: A sends 0.5 NOUS to B with 0.001 NOUS fee.
	fee := int64(100_000) // 0.001 NOUS
	addrB := walletB.GetAddress()
	nextHeight := chain.Height + 1
	transfer, err := walletA.CreateTransaction(addrB, NOU/2, fee, chain.UTXOSet, nextHeight)
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	// Mine block 102 including the transfer tx.
	blk102, err := consensus.MineBlock(prev, []*tx.Transaction{transfer}, pkhA, chain.Difficulty, nextHeight, chain.UTXOSet, false)
	if err != nil {
		t.Fatalf("mine block 102: %v", err)
	}
	if err := chain.AddBlock(blk102); err != nil {
		t.Fatalf("add block 102: %v", err)
	}

	// Verify B balance = 0.5 NOUS.
	balB := chain.UTXOSet.GetBalance(pkhB)
	if balB != NOU/2 {
		t.Fatalf("B balance: want %d, got %d", NOU/2, balB)
	}

	// Verify A balance.
	// 102 blocks × 1 NOUS = 102 NOUS total mined, minus 0.5 sent to B.
	// Fee (0.001) paid by A is recovered as miner of block 102.
	expectedA := int64(102)*NOU - NOU/2
	balA = chain.UTXOSet.GetBalance(pkhA)
	if balA != expectedA {
		t.Fatalf("A balance: want %d, got %d", expectedA, balA)
	}

	_ = pubA // used for mining key derivation
}

// ============================================================
// 3. TestTwoNodeSync
// ============================================================

func TestTwoNodeSync(t *testing.T) {
	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pubKey.SerializeCompressed())

	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60, 0x1d00ffff, false)

	// --- Node A: mine 3 blocks ---
	chainA := consensus.NewChainState(genesis)
	chainA.Difficulty = easyDifficulty()
	chainA.Anchor.Target = easyDifficulty().PoWTarget

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

	reasonerA := node.NewReasoner(chainA, serverA, storeA, pubKey, new(sync.Mutex))

	prev := &genesis.Header
	for h := uint64(1); h <= 3; h++ {
		blk, err := consensus.MineBlock(prev, nil, pubKeyHash, chainA.Difficulty, h, nil, false)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := reasonerA.ApplyBlock(blk); err != nil {
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
	chainB.Anchor.Target = easyDifficulty().PoWTarget

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

	reasonerB := node.NewReasoner(chainB, serverB, storeB, pubKey, new(sync.Mutex))

	for h := uint64(1); h <= 3; h++ {
		blk, err := storeA.LoadBlockByHeight(h)
		if err != nil {
			t.Fatalf("load block %d from A: %v", h, err)
		}
		if err := reasonerB.ApplyBlock(blk); err != nil {
			t.Fatalf("apply block %d to B: %v", h, err)
		}
	}

	if chainB.Height != 3 {
		t.Fatalf("node B height: want 3, got %d", chainB.Height)
	}

	tipA := chainA.Tip.Hash()
	tipB := chainB.Tip.Hash()
	if tipA != tipB {
		t.Fatalf("tip hash mismatch: A=%x B=%x", tipA[:8], tipB[:8])
	}
}

// ============================================================
// 4. TestMiningWithSAT
// ============================================================

func TestMiningWithSAT(t *testing.T) {
	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pubKey.SerializeCompressed())

	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60, 0x1d00ffff, false)
	chain := consensus.NewChainState(genesis)
	chain.Difficulty = easyDifficulty()
	chain.Anchor.Target = easyDifficulty().PoWTarget

	// Mine a single block.
	blk, err := consensus.MineBlock(&genesis.Header, nil, pubKeyHash, chain.Difficulty, 1, nil, false)
	if err != nil {
		t.Fatalf("mine block: %v", err)
	}
	if err := chain.AddBlock(blk); err != nil {
		t.Fatalf("add block: %v", err)
	}

	// Block must contain a valid SAT solution.
	if len(blk.SATSolution) == 0 {
		t.Fatal("block has no SAT solution")
	}
	if len(blk.SATSolution) != consensus.SATVariables {
		t.Fatalf("SAT solution length: want %d, got %d", consensus.SATVariables, len(blk.SATSolution))
	}
}
