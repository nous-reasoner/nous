// nousd is the NOUS full node daemon.
//
// Usage:
//
//	nousd [flags]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
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

const version = "0.2.0-dev"

func main() {
	// CLI flags.
	dataDir := flag.String("datadir", defaultDataDir(), "data directory")
	port := flag.Int("port", network.DefaultPort, "P2P listen port")
	rpcPort := flag.Int("rpcport", 9332, "JSON-RPC listen port")
	seeds := flag.String("seeds", "", "comma-separated seed node addresses")
	reason := flag.Bool("reason", false, "enable reasoning (mining)")
	keyFile := flag.String("key", "", "path to wallet file for reasoning")
	password := flag.String("password", "", "wallet password")
	testnet := flag.Bool("testnet", false, "use testnet")
	logLevel := flag.String("loglevel", "info", "log level (debug, info, warn, error)")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("nousd %s\n", version)
		return
	}

	_ = logLevel // reserved for future use

	log.Printf("nousd %s starting...", version)
	log.Printf("data directory: %s", *dataDir)

	// Initialize block store.
	store, err := storage.NewBlockStore(*dataDir)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	// Load or create genesis block.
	var genesis *block.Block
	if store.HasBlock(0) {
		// Existing data — load stored genesis.
		genesis, err = store.LoadBlockByHeight(0)
		if err != nil {
			log.Fatalf("load genesis: %v", err)
		}
		gh := genesis.Header.Hash()
		log.Printf("genesis loaded: %x", gh[:8])
	} else {
		// Fresh start — create and save genesis.
		genesisPKH := make([]byte, 20) // zero hash for genesis
		genesisBits := uint32(0x1d00ffff) // mainnet
		if *testnet {
			genesisBits = uint32(0x2000ffff) // testnet
		}
		var genesisTimestamp uint32
		if *testnet {
			genesisTimestamp = 1772465400 // testnet genesis: 2026-03-01 (~fixed)
		}
		genesis = block.GenesisBlock(genesisPKH, genesisTimestamp, genesisBits)
		if err := store.SaveBlock(genesis, 0); err != nil {
			log.Fatalf("save genesis: %v", err)
		}
		genesisHash := genesis.Header.Hash()
		store.SaveChainTip(storage.ChainTip{Hash: genesisHash, Height: 0})
		log.Printf("genesis block created: %x (timestamp=%d)", genesisHash[:8], genesisTimestamp)
	}

	// Open BoltDB-backed UTXO set.
	utxoDBPath := filepath.Join(*dataDir, "utxo.db")
	boltUTXO, err := tx.NewBoltUTXOSet(utxoDBPath)
	if err != nil {
		log.Fatalf("utxo: %v", err)
	}
	utxoPopulated := boltUTXO.Count() > 0

	// Initialize chain state.
	var chain *consensus.ChainState
	tip, tipErr := store.GetChainTip()

	if utxoPopulated {
		// UTXO set persisted from previous run — skip UTXO replay.
		chain = consensus.NewChainStateRestored(genesis, boltUTXO)
		if tipErr == nil && tip.Height > 0 {
			log.Printf("recovering block index from %d stored blocks (UTXO set loaded from disk)...", tip.Height)
			for h := uint64(1); h <= tip.Height; h++ {
				blk, err := store.LoadBlockByHeight(h)
				if err != nil {
					log.Fatalf("recovery: load block %d: %v", h, err)
				}
				if err := chain.AddBlockIndexOnly(blk); err != nil {
					log.Fatalf("recovery: index block %d: %v", h, err)
				}
			}
			log.Printf("block index recovered: height=%d", chain.Height)
		}
	} else {
		// Fresh UTXO set — apply genesis + all stored blocks.
		chain = consensus.NewChainStateWithUTXO(genesis, boltUTXO)
		if tipErr == nil && tip.Height > 0 {
			log.Printf("recovering chain state from %d stored blocks...", tip.Height)
			for h := uint64(1); h <= tip.Height; h++ {
				blk, err := store.LoadBlockByHeight(h)
				if err != nil {
					log.Fatalf("recovery: load block %d: %v", h, err)
				}
				if err := chain.AddBlockUnchecked(blk); err != nil {
					log.Fatalf("recovery: apply block %d: %v", h, err)
				}
			}
			log.Printf("chain state recovered: height=%d", chain.Height)
		}
	}

	// Configure P2P network.
	magic := network.MainNetMagic
	if *testnet {
		magic = network.TestNetMagic
	}
	var seedList []string
	if *seeds != "" {
		seedList = splitSeeds(*seeds)
	}
	netCfg := network.ServerConfig{
		ListenAddr: fmt.Sprintf(":%d", *port),
		Magic:      magic,
		Seeds:      seedList,
	}
	server := network.NewServer(netCfg)

	// Start P2P server.
	if err := server.Start(); err != nil {
		log.Fatalf("p2p: %v", err)
	}
	server.SetBlockHeight(chain.Height)
	log.Printf("p2p: listening on %s", server.ListenAddr())

	// Shared mutex for ChainState access (used by both ChainAdapter and Reasoner).
	var chainMu sync.Mutex

	// Start block syncer (handles incoming blocks and sync protocol).
	chainAdapter := node.NewChainAdapter(chain, store, &chainMu)
	syncer := network.NewBlockSyncer(server, chainAdapter)
	syncer.Start()

	// Periodically trigger sync from best peer.
	syncQuit := make(chan struct{})
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-syncQuit:
				return
			case <-ticker.C:
				syncer.TriggerSync()
			}
		}
	}()

	// Start reasoner if enabled.
	var reasoner *node.Reasoner
	if *reason {
		_, pubKey, err := loadKey(*keyFile, *password)
		if err != nil {
			log.Fatalf("key: %v", err)
		}
		reasoner = node.NewReasoner(chain, server, store, pubKey, &chainMu)
		reasoner.Start()
		log.Println("reasoning enabled")
	}

	// Start RPC server.
	rpcAddr := fmt.Sprintf(":%d", *rpcPort)
	rpc := node.NewRPCServer(rpcAddr, chain, server, store, reasoner)
	rpc.Start()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("received %s, shutting down...", sig)

	// Graceful shutdown: sync → reasoner → RPC → P2P → save state.
	close(syncQuit)
	if reasoner != nil {
		reasoner.Stop()
		log.Println("reasoner stopped")
	}
	rpc.Stop()
	log.Println("rpc stopped")
	server.Stop()
	log.Println("p2p stopped")

	// Save final chain tip.
	tipHash := chain.Tip.Hash()
	store.SaveChainTip(storage.ChainTip{Hash: tipHash, Height: chain.Height})
	log.Printf("chain tip saved: height=%d hash=%x", chain.Height, tipHash[:8])

	// Close UTXO database.
	if err := boltUTXO.Close(); err != nil {
		log.Printf("utxo close: %v", err)
	}
	log.Println("utxo db closed")

	log.Println("nousd shutdown complete")
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".nous"
	}
	return home + "/.nous"
}

func splitSeeds(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			part := trimSpace(s[start:i])
			if part != "" {
				result = append(result, part)
			}
			start = i + 1
		}
	}
	part := trimSpace(s[start:])
	if part != "" {
		result = append(result, part)
	}
	return result
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && s[i] == ' ' {
		i++
	}
	for j > i && s[j-1] == ' ' {
		j--
	}
	return s[i:j]
}

func loadKey(keyFile, password string) (*crypto.PrivateKey, *crypto.PublicKey, error) {
	if keyFile != "" {
		w, err := wallet.LoadFromFile(keyFile, password)
		if err != nil {
			return nil, nil, fmt.Errorf("load wallet: %w", err)
		}
		kp := w.Keys[w.Primary]
		return kp.PrivateKey, kp.PublicKey, nil
	}
	// No wallet file — generate an ephemeral key.
	log.Println("warning: no wallet specified, using ephemeral key")
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}
