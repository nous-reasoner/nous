// NOUS Reasoner Web Worker
// Runs WASM in a separate thread so the UI stays responsive.

importScripts('wasm_exec.js');

// Bridge: Go WASM calls postReasonerLog → worker sends to main thread.
self.postReasonerLog = function(msg) {
  self.postMessage({ type: 'log', msg: msg });
};

let wasmReady = false;

async function loadWasm() {
  const go = new Go();
  try {
    const result = await WebAssembly.instantiateStreaming(fetch('miner.wasm'), go.importObject);
    go.run(result.instance);
    wasmReady = true;
    self.postMessage({ type: 'ready' });
  } catch(e) {
    self.postMessage({ type: 'error', msg: 'WASM load failed: ' + e.message });
  }
}

self.onmessage = function(e) {
  const { action, nodeUrl, address } = e.data;

  if (!wasmReady && action !== 'load') {
    self.postMessage({ type: 'error', msg: 'WASM not ready' });
    return;
  }

  switch (action) {
    case 'load':
      loadWasm();
      break;

    case 'start':
      const result = nousReasoner.start(nodeUrl, address);
      self.postMessage({ type: 'started', result: result });
      break;

    case 'stop':
      nousReasoner.stop();
      self.postMessage({ type: 'stopped' });
      break;

    case 'stats':
      try {
        const stats = nousReasoner.getStats();
        self.postMessage({ type: 'stats', data: stats });
      } catch(err) {
        self.postMessage({ type: 'stats', data: '{}' });
      }
      break;

    case 'createWallet':
      const wallet = nousReasoner.createWallet();
      self.postMessage({ type: 'wallet', data: {
        private_key: wallet.private_key,
        public_key: wallet.public_key,
        address: wallet.address,
        error: wallet.error || null
      }});
      break;

    case 'getBalance':
      nousReasoner.getBalance(nodeUrl, address).then(function(bal) {
        self.postMessage({ type: 'balance', data: { balance: bal.balance, immature: bal.immature } });
      }).catch(function() {
        self.postMessage({ type: 'balance', data: null });
      });
      break;
  }
};
