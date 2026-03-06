package node

import (
	"sync"
	"testing"
	"time"

	"nous/block"
	"nous/consensus"
	"nous/crypto"
	"nous/network"
	"nous/storage"
	"nous/tx"
	"nous/wallet"
)

// easyDifficulty is defined in node_test.go.

const nou = int64(1_0000_0000)

// easyGenesis creates a testnet genesis block with easy difficulty.
func easyGenesis(pkh []byte) *block.Block {
	return block.GenesisBlock(pkh, uint32(time.Now().Unix())-60, consensus.TargetToCompact(easyDifficulty().PoWTarget), true)
}

// mineChain mines n blocks on top of chain using the given key, returns the last header.
func mineChain(t *testing.T, chain *consensus.ChainState, prev *block.Header, pkh []byte, n int) *block.Header {
	t.Helper()
	startHeight := chain.Height + 1
	for h := startHeight; h < startHeight+uint64(n); h++ {
		blk, err := consensus.MineBlock(prev, nil, pkh, chain.Difficulty, h, nil, true)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header
	}
	return prev
}

// TestFullLifecycle is the main integration test suite with five scenarios.
func TestFullLifecycle(t *testing.T) {
	t.Run("Scenario1_MiningAndSync", testMiningAndSync)
	t.Run("Scenario2_Transfer", testTransfer)
	t.Run("Scenario3_ChainReorg", testChainReorg)
	t.Run("Scenario4_DisconnectRecovery", testDisconnectRecovery)
	t.Run("Scenario5_InvalidBlockRejection", testInvalidBlockRejection)
}

// ============================================================
// Scenario 1: Basic mining and two-node sync over the network
// ============================================================
func testMiningAndSync(t *testing.T) {
	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkh := crypto.Hash160(pubKey.SerializeCompressed())

	genesis := easyGenesis(pkh)

	// --- Node A: mine 5 blocks ---
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

	chainMuA := new(sync.Mutex)
	cfgA := network.ServerConfig{ListenAddr: ":0", Magic: network.TestNetMagic}
	serverA := network.NewServer(cfgA)
	if err := serverA.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverA.Stop()

	adapterA := NewChainAdapter(chainA, storeA, chainMuA)
	syncerA := network.NewBlockSyncer(serverA, adapterA)
	syncerA.Start()

	// Mine 5 blocks on A.
	prev := &genesis.Header
	for h := uint64(1); h <= 5; h++ {
		chainMuA.Lock()
		blk, err := consensus.MineBlock(prev, nil, pkh, chainA.Difficulty, h, nil, true)
		if err != nil {
			chainMuA.Unlock()
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chainA.AddBlock(blk); err != nil {
			chainMuA.Unlock()
			t.Fatalf("add block %d on A: %v", h, err)
		}
		if err := storeA.SaveBlock(blk, h); err != nil {
			chainMuA.Unlock()
			t.Fatalf("save block %d: %v", h, err)
		}
		serverA.SetBlockHeight(h)
		chainMuA.Unlock()
		prev = &blk.Header
	}

	if chainA.Height != 5 {
		t.Fatalf("node A height: want 5, got %d", chainA.Height)
	}

	// --- Node B: start empty, connect to A, sync via P2P ---
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

	chainMuB := new(sync.Mutex)
	cfgB := network.ServerConfig{ListenAddr: ":0", Magic: network.TestNetMagic}
	serverB := network.NewServer(cfgB)
	if err := serverB.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverB.Stop()

	adapterB := NewChainAdapter(chainB, storeB, chainMuB)
	syncerB := network.NewBlockSyncer(serverB, adapterB)
	syncerB.Start()

	// Connect B to A.
	addrA := serverA.ListenAddr()
	if err := serverB.Connect(addrA); err != nil {
		t.Fatalf("connect B→A: %v", err)
	}

	// Wait for handshake.
	time.Sleep(500 * time.Millisecond)

	// Trigger sync from B.
	syncerB.TriggerSync()

	// Wait for sync to complete.
	err = syncerB.WaitForSync(5, 10*time.Second)
	if err != nil {
		t.Fatalf("sync B: %v", err)
	}

	// Verify both nodes agree.
	if adapterB.Height() != 5 {
		t.Fatalf("node B height: want 5, got %d", adapterB.Height())
	}
	tipA := adapterA.TipHash()
	tipB := adapterB.TipHash()
	if tipA != tipB {
		t.Fatalf("tip hash mismatch: A=%x B=%x", tipA[:8], tipB[:8])
	}
}

// ============================================================
// Scenario 2: Transfer (mine to maturity, send 2 nouS, verify balances)
// ============================================================
func testTransfer(t *testing.T) {
	walletA, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	walletA.IsTestnet = true

	walletB, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	walletB.IsTestnet = true

	pkhA := walletA.PubKeyHash()
	pkhB := walletB.PubKeyHash()

	genesis := easyGenesis(pkhA)
	chain := consensus.NewChainState(genesis)
	chain.Difficulty = easyDifficulty()
	chain.Anchor.Target = easyDifficulty().PoWTarget
	chain.IsTestnet = true

	// Mine 101 blocks so early coinbase outputs mature.
	// Testnet CoinbaseMaturity=10, so genesis coinbase matures at height 10.
	prev := &genesis.Header
	for h := uint64(1); h <= 101; h++ {
		blk, err := consensus.MineBlock(prev, nil, pkhA, chain.Difficulty, h, nil, true)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header
	}

	if chain.Height != 101 {
		t.Fatalf("height: want 101, got %d", chain.Height)
	}

	// Verify A has all mined coins (101 blocks × 1 nouS + genesis testnet 1 nouS).
	balA := chain.UTXOSet.GetBalance(pkhA)
	// Genesis gives 1 nouS (testnet) + 101 mined blocks × 1 nouS = 102 nouS.
	expectedTotal := int64(102) * nou
	if balA != expectedTotal {
		t.Fatalf("A balance before transfer: want %d, got %d", expectedTotal, balA)
	}

	// Create transfer: A sends 2 nouS to B with 0.001 nouS fee.
	fee := int64(100_000) // 0.001 nouS
	addrB := walletB.GetAddress()
	nextHeight := chain.Height + 1
	transfer, err := walletA.CreateTransaction(addrB, 2*nou, fee, chain.UTXOSet, nextHeight)
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	// Mine block including the transfer.
	blk, err := consensus.MineBlock(prev, []*tx.Transaction{transfer}, pkhA, chain.Difficulty, nextHeight, chain.UTXOSet, true)
	if err != nil {
		t.Fatalf("mine transfer block: %v", err)
	}
	if err := chain.AddBlock(blk); err != nil {
		t.Fatalf("add transfer block: %v", err)
	}

	// Verify B received 2 nouS.
	balB := chain.UTXOSet.GetBalance(pkhB)
	if balB != 2*nou {
		t.Fatalf("B balance: want %d, got %d", 2*nou, balB)
	}

	// Verify A balance: mined 103 total (102 before + 1 from transfer block), minus 2 nouS sent.
	// Fee paid by A is recovered as miner of the transfer block.
	expectedA := int64(103)*nou - 2*nou
	balA = chain.UTXOSet.GetBalance(pkhA)
	if balA != expectedA {
		t.Fatalf("A balance after transfer: want %d, got %d", expectedA, balA)
	}
}

// ============================================================
// Scenario 3: Chain reorg (shorter chain A replaced by heavier chain B)
// ============================================================
func testChainReorg(t *testing.T) {
	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkh := crypto.Hash160(pubKey.SerializeCompressed())

	genesis := easyGenesis(pkh)
	chain := consensus.NewChainState(genesis)
	chain.Difficulty = easyDifficulty()
	chain.Anchor.Target = easyDifficulty().PoWTarget
	chain.IsTestnet = true

	// Build chain A: 3 blocks on top of genesis.
	prevA := &genesis.Header
	var chainABlocks []*block.Block
	for h := uint64(1); h <= 3; h++ {
		blk, err := consensus.MineBlock(prevA, nil, pkh, chain.Difficulty, h, nil, true)
		if err != nil {
			t.Fatalf("mine chain A block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add chain A block %d: %v", h, err)
		}
		chainABlocks = append(chainABlocks, blk)
		prevA = &blk.Header
	}

	if chain.Height != 3 {
		t.Fatalf("after chain A: height want 3, got %d", chain.Height)
	}
	tipAfterA := chain.Tip.Hash()

	// Build chain B: 5 blocks on top of genesis (in a separate chain state,
	// then feed them to the main chain to trigger reorg).
	chainB := consensus.NewChainState(genesis)
	chainB.Difficulty = easyDifficulty()
	chainB.Anchor.Target = easyDifficulty().PoWTarget
	chainB.IsTestnet = true

	// Use a different key for chain B so block hashes differ from chain A.
	_, pubKeyB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkhB := crypto.Hash160(pubKeyB.SerializeCompressed())

	prevB := &genesis.Header
	var chainBBlocks []*block.Block
	for h := uint64(1); h <= 5; h++ {
		blk, err := consensus.MineBlock(prevB, nil, pkhB, chainB.Difficulty, h, nil, true)
		if err != nil {
			t.Fatalf("mine chain B block %d: %v", h, err)
		}
		if err := chainB.AddBlock(blk); err != nil {
			t.Fatalf("add chain B block %d (to staging): %v", h, err)
		}
		chainBBlocks = append(chainBBlocks, blk)
		prevB = &blk.Header
	}

	// Feed chain B blocks to the main chain — should trigger a reorg.
	for i, blk := range chainBBlocks {
		err := chain.AddBlock(blk)
		if err != nil {
			t.Fatalf("add chain B block %d to main: %v", i+1, err)
		}
	}

	// After reorg, height should be 5 (chain B is longer).
	if chain.Height != 5 {
		t.Fatalf("after reorg: height want 5, got %d", chain.Height)
	}

	// Tip should have changed from chain A.
	tipAfterReorg := chain.Tip.Hash()
	if tipAfterReorg == tipAfterA {
		t.Fatal("tip didn't change after reorg")
	}

	// Tip should match chain B's tip.
	expectedTip := chainBBlocks[4].Header.Hash()
	if tipAfterReorg != expectedTip {
		t.Fatalf("after reorg: tip want %x, got %x", expectedTip[:8], tipAfterReorg[:8])
	}

	// UTXO set should reflect chain B (miner B's key, not A's).
	// Chain B: genesis (1 nouS to pkh) + 5 blocks (1 nouS each to pkhB) = pkhB has 5 nouS.
	balB := chain.UTXOSet.GetBalance(pkhB)
	if balB != 5*nou {
		t.Fatalf("reorg: pkhB balance want %d, got %d", 5*nou, balB)
	}

	// pkh (chain A miner) should have only genesis UTXO (chain A blocks rolled back).
	balA := chain.UTXOSet.GetBalance(pkh)
	if balA != 1*nou { // only genesis testnet reward
		t.Fatalf("reorg: pkh balance want %d, got %d", 1*nou, balA)
	}

	_ = chainABlocks // used for building chain A
}

// ============================================================
// Scenario 4: Disconnect recovery (sync → disconnect → mine → reconnect → sync)
// ============================================================
func testDisconnectRecovery(t *testing.T) {
	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkh := crypto.Hash160(pubKey.SerializeCompressed())

	genesis := easyGenesis(pkh)

	// --- Node A: mine initial 5 blocks ---
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

	chainMuA := new(sync.Mutex)
	cfgA := network.ServerConfig{ListenAddr: ":0", Magic: network.TestNetMagic}
	serverA := network.NewServer(cfgA)
	if err := serverA.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverA.Stop()

	adapterA := NewChainAdapter(chainA, storeA, chainMuA)
	syncerA := network.NewBlockSyncer(serverA, adapterA)
	syncerA.Start()

	// Mine 5 blocks on A.
	prev := &genesis.Header
	for h := uint64(1); h <= 5; h++ {
		chainMuA.Lock()
		blk, err := consensus.MineBlock(prev, nil, pkh, chainA.Difficulty, h, nil, true)
		if err != nil {
			chainMuA.Unlock()
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chainA.AddBlock(blk); err != nil {
			chainMuA.Unlock()
			t.Fatalf("add block %d on A: %v", h, err)
		}
		if err := storeA.SaveBlock(blk, h); err != nil {
			chainMuA.Unlock()
			t.Fatalf("save block %d: %v", h, err)
		}
		serverA.SetBlockHeight(h)
		chainMuA.Unlock()
		prev = &blk.Header
	}

	// --- Node B: sync from A ---
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

	chainMuB := new(sync.Mutex)
	cfgB := network.ServerConfig{ListenAddr: ":0", Magic: network.TestNetMagic}
	serverB := network.NewServer(cfgB)
	if err := serverB.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverB.Stop()

	adapterB := NewChainAdapter(chainB, storeB, chainMuB)
	syncerB := network.NewBlockSyncer(serverB, adapterB)
	syncerB.Start()

	// Connect B to A and sync.
	addrA := serverA.ListenAddr()
	if err := serverB.Connect(addrA); err != nil {
		t.Fatalf("connect B→A: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	syncerB.TriggerSync()

	if err := syncerB.WaitForSync(5, 10*time.Second); err != nil {
		t.Fatalf("initial sync B: %v", err)
	}

	if adapterB.Height() != 5 {
		t.Fatalf("B height after initial sync: want 5, got %d", adapterB.Height())
	}

	// --- Disconnect: close all peers on B ---
	for _, p := range serverB.Peers().All() {
		p.Close()
	}
	time.Sleep(300 * time.Millisecond)

	// Mine 10 more blocks on A while disconnected.
	for h := uint64(6); h <= 15; h++ {
		chainMuA.Lock()
		blk, err := consensus.MineBlock(prev, nil, pkh, chainA.Difficulty, h, nil, true)
		if err != nil {
			chainMuA.Unlock()
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chainA.AddBlock(blk); err != nil {
			chainMuA.Unlock()
			t.Fatalf("add block %d on A: %v", h, err)
		}
		if err := storeA.SaveBlock(blk, h); err != nil {
			chainMuA.Unlock()
			t.Fatalf("save block %d: %v", h, err)
		}
		serverA.SetBlockHeight(h)
		chainMuA.Unlock()
		prev = &blk.Header
	}

	// B should still be at 5.
	if adapterB.Height() != 5 {
		t.Fatalf("B height while disconnected: want 5, got %d", adapterB.Height())
	}

	// --- Reconnect: B connects to A again ---
	if err := serverB.Connect(addrA); err != nil {
		t.Fatalf("reconnect B→A: %v", err)
	}

	// Wait for handshake, then sync directly (bypassing TriggerSync cooldown).
	var syncPeer *network.Peer
	deadline := time.After(5 * time.Second)
	for syncPeer == nil {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for handshake after reconnect")
		default:
		}
		for _, p := range serverB.Peers().All() {
			if p.Handshaked {
				syncPeer = p
				break
			}
		}
		if syncPeer == nil {
			time.Sleep(50 * time.Millisecond)
		}
	}

	if err := syncerB.SyncFromPeer(syncPeer); err != nil {
		t.Fatalf("sync from peer: %v", err)
	}

	if err := syncerB.WaitForSync(15, 10*time.Second); err != nil {
		t.Fatalf("resync B: %v", err)
	}

	// Verify B caught up.
	if adapterB.Height() != 15 {
		t.Fatalf("B height after reconnect: want 15, got %d", adapterB.Height())
	}

	// Verify tips match.
	tipA := adapterA.TipHash()
	tipB := adapterB.TipHash()
	if tipA != tipB {
		t.Fatalf("tip mismatch after reconnect: A=%x B=%x", tipA[:8], tipB[:8])
	}
}

// ============================================================
// Scenario 5: Invalid block rejection (bad DifficultyBits)
// ============================================================
func testInvalidBlockRejection(t *testing.T) {
	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkh := crypto.Hash160(pubKey.SerializeCompressed())

	genesis := easyGenesis(pkh)
	chain := consensus.NewChainState(genesis)
	chain.Difficulty = easyDifficulty()
	chain.Anchor.Target = easyDifficulty().PoWTarget
	chain.IsTestnet = true

	// Mine one valid block first.
	blk1, err := consensus.MineBlock(&genesis.Header, nil, pkh, chain.Difficulty, 1, nil, true)
	if err != nil {
		t.Fatalf("mine block 1: %v", err)
	}
	if err := chain.AddBlock(blk1); err != nil {
		t.Fatalf("add block 1: %v", err)
	}

	if chain.Height != 1 {
		t.Fatalf("height after block 1: want 1, got %d", chain.Height)
	}

	// Mine a valid block 2 using the real difficulty, then corrupt it.
	blk2, err := consensus.MineBlock(&blk1.Header, nil, pkh, chain.Difficulty, 2, nil, true)
	if err != nil {
		t.Fatalf("mine block 2: %v", err)
	}

	// Corrupt the DifficultyBits to a different value.
	blk2.Header.DifficultyBits = 0x1d00ffff // mainnet difficulty, not matching easy target

	// Attempt to add the corrupted block.
	err = chain.AddBlock(blk2)
	if err == nil {
		t.Fatal("expected error when adding block with wrong DifficultyBits, got nil")
	}
	t.Logf("correctly rejected invalid block: %v", err)

	// Chain should be unchanged.
	if chain.Height != 1 {
		t.Fatalf("height after invalid block: want 1, got %d", chain.Height)
	}

	// Can still mine a valid block on top.
	blk2Valid, err := consensus.MineBlock(&blk1.Header, nil, pkh, chain.Difficulty, 2, nil, true)
	if err != nil {
		t.Fatalf("mine valid block 2: %v", err)
	}
	if err := chain.AddBlock(blk2Valid); err != nil {
		t.Fatalf("add valid block 2: %v", err)
	}

	if chain.Height != 2 {
		t.Fatalf("height after valid block 2: want 2, got %d", chain.Height)
	}
}
