// DEPRECATED: Browser miner retired 2026-03-15. See docs.nouschain.org for mining guide.
// NOUS Reasoner Web Worker
// Each worker loads its own WASM instance for parallel solving.

const WASM_VERSION = '2.0.0';
const WASM_CACHE_KEY = 'miner-' + WASM_VERSION;

importScripts('wasm_exec.js');

// Bridge: Go WASM calls postReasonerLog -> worker sends to main thread.
self.postReasonerLog = function(msg) {
  self.postMessage({ type: 'log', msg: msg });
};

let wasmReady = false;

async function loadWasm() {
  const go = new Go();
  try {
    let result;

    // Try loading from IndexedDB cache first.
    const cachedBytes = await getCachedWasm();
    if (cachedBytes) {
      self.postMessage({ type: 'log', msg: 'Loading from cache...' });
      result = await WebAssembly.instantiate(cachedBytes, go.importObject);
    } else {
      // Fetch with progress tracking.
      self.postMessage({ type: 'log', msg: 'Downloading WASM (~3MB)...' });
      const resp = await fetch('miner.wasm');
      const reader = resp.body.getReader();
      const contentLength = +resp.headers.get('Content-Length') || 3400000;
      let received = 0;
      const chunks = [];

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
        received += value.length;
        const pct = Math.min(99, Math.round(received / contentLength * 100));
        self.postMessage({ type: 'progress', pct: pct });
      }

      const bytes = new Uint8Array(received);
      let pos = 0;
      for (const chunk of chunks) {
        bytes.set(chunk, pos);
        pos += chunk.length;
      }

      self.postMessage({ type: 'progress', pct: 100 });
      self.postMessage({ type: 'log', msg: 'Compiling WASM...' });
      result = await WebAssembly.instantiate(bytes, go.importObject);

      // Cache for next time.
      cacheWasm(bytes).catch(function() {});
    }

    go.run(result.instance);
    wasmReady = true;
    self.postMessage({ type: 'ready' });
  } catch(e) {
    self.postMessage({ type: 'error', msg: 'WASM load failed: ' + e.message });
  }
}

// --- IndexedDB cache ---
function openCacheDB() {
  return new Promise(function(resolve, reject) {
    const req = indexedDB.open('nous-wasm-cache', 1);
    req.onupgradeneeded = function() { req.result.createObjectStore('wasm'); };
    req.onsuccess = function() { resolve(req.result); };
    req.onerror = function() { reject(req.error); };
  });
}

async function getCachedWasm() {
  try {
    const db = await openCacheDB();
    return new Promise(function(resolve) {
      const tx = db.transaction('wasm', 'readonly');
      const req = tx.objectStore('wasm').get(WASM_CACHE_KEY);
      req.onsuccess = function() { resolve(req.result || null); };
      req.onerror = function() { resolve(null); };
    });
  } catch(e) { return null; }
}

async function cacheWasm(bytes) {
  const db = await openCacheDB();
  const tx = db.transaction('wasm', 'readwrite');
  tx.objectStore('wasm').put(bytes, WASM_CACHE_KEY);
}

self.onmessage = function(e) {
  var d = e.data;

  if (!wasmReady && d.action !== 'load') {
    self.postMessage({ type: 'error', msg: 'WASM not ready' });
    return;
  }

  switch (d.action) {
    case 'load':
      loadWasm();
      break;

    case 'solveBatch':
      var result = nousReasoner.solveBatch(d.workJSON, d.seedStart, d.seedEnd);
      self.postMessage({ type: 'batchResult', data: result });
      break;

    case 'createWallet':
      var wallet = nousReasoner.createWallet();
      self.postMessage({ type: 'wallet', data: {
        private_key: wallet.private_key,
        public_key: wallet.public_key,
        address: wallet.address,
        error: wallet.error || null
      }});
      break;

    case 'getBalance':
      nousReasoner.getBalance(d.nodeUrl, d.address).then(function(bal) {
        self.postMessage({ type: 'balance', data: { balance: bal.balance, immature: bal.immature } });
      }).catch(function() {
        self.postMessage({ type: 'balance', data: null });
      });
      break;
  }
};
