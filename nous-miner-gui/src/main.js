const { app, BrowserWindow, ipcMain, dialog, Menu } = require('electron');
const path = require('path');
const { spawn } = require('child_process');
const fs = require('fs');
const os = require('os');

let mainWindow;
let minerProcess;

// --- Wallet Backend Process ---
let walletBackend;
let walletRequestId = 0;
const walletPending = new Map();

function startWalletBackend() {
  const binaryName = process.platform === 'win32' ? 'wallet-backend.exe' : 'wallet-backend';
  const basePath = app.isPackaged
    ? path.join(process.resourcesPath, 'app.asar.unpacked')
    : path.join(__dirname, '..');
  const binaryPath = path.join(basePath, 'backend', binaryName);

  walletBackend = spawn(binaryPath, [], { stdio: ['pipe', 'pipe', 'pipe'] });

  let buffer = '';
  walletBackend.stdout.on('data', (data) => {
    buffer += data.toString();
    const lines = buffer.split('\n');
    buffer = lines.pop();
    for (const line of lines) {
      if (!line.trim()) continue;
      try {
        const resp = JSON.parse(line);
        const resolve = walletPending.get(resp.id);
        if (resolve) {
          walletPending.delete(resp.id);
          resolve(resp);
        }
      } catch (e) { /* ignore */ }
    }
  });

  walletBackend.stderr.on('data', (data) => {
    console.log('[wallet-backend]', data.toString().trim());
  });

  walletBackend.on('error', (err) => {
    console.error('[wallet-backend] spawn error:', err.message);
  });

  walletBackend.on('close', (code, signal) => {
    console.log('wallet-backend exited, code:', code, 'signal:', signal);
    walletBackend = null;
  });
}

function callWalletBackend(method, params = {}) {
  return new Promise((resolve, reject) => {
    if (!walletBackend) {
      reject(new Error('wallet backend not running'));
      return;
    }
    const id = ++walletRequestId;
    walletPending.set(id, resolve);
    const req = JSON.stringify({ method, params, id }) + '\n';
    walletBackend.stdin.write(req);

    setTimeout(() => {
      if (walletPending.has(id)) {
        walletPending.delete(id);
        reject(new Error('timeout'));
      }
    }, 30000);
  });
}

// --- Window ---

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 900,
    height: 750,
    webPreferences: {
      nodeIntegration: true,
      contextIsolation: false
    }
  });

  mainWindow.loadFile('public/index.html');
  Menu.setApplicationMenu(null);
}

app.whenReady().then(() => {
  startWalletBackend();
  createWindow();
});

app.on('window-all-closed', () => {
  if (minerProcess) {
    minerProcess.removeAllListeners();
    minerProcess.kill();
    minerProcess = null;
  }
  if (walletBackend) {
    walletBackend.kill();
    walletBackend = null;
  }
  mainWindow = null;
  if (process.platform !== 'darwin') app.quit();
});

// --- Miner IPC handlers ---

ipcMain.handle('start-mining', async (event, config) => {
  if (minerProcess) return { error: 'Already mining' };

  const minerExt = process.platform === 'win32' ? '.exe' : '';
  const basePath = app.isPackaged
    ? path.join(process.resourcesPath, 'app.asar.unpacked')
    : path.join(__dirname, '..');
  const minerPath = path.join(basePath, 'backend', 'miner' + minerExt);
  const args = [
    '--node', config.nodeUrl,
    '--address', config.address,
    '--solver', config.solver || 'probsat'
  ];

  if (config.solver === 'ai-guided' || config.solver === 'pure-ai') {
    if (config.aiProvider) args.push('--ai-provider', config.aiProvider);
    if (config.apiKey) args.push('--api-key', config.apiKey);
    if (config.model) args.push('--model', config.model);
    if (config.baseUrl) args.push('--base-url', config.baseUrl);
  }

  if (config.solver === 'custom' && config.scriptPath) {
    args.push('--script', config.scriptPath);
  }

  minerProcess = spawn(minerPath, args);

  minerProcess.stdout.on('data', (data) => {
    const log = data.toString();
    console.log('[Miner]', log);
    if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.send('miner-log', log);
  });

  minerProcess.stderr.on('data', (data) => {
    const log = data.toString();
    console.error('[Miner Error]', log);
    if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.send('miner-log', log);
  });

  minerProcess.on('error', (err) => {
    console.error('[Miner Process Error]', err);
    if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.send('miner-log', '[FATAL] ' + err.message);
    minerProcess = null;
  });

  minerProcess.on('close', () => {
    minerProcess = null;
    if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.send('miner-stopped');
  });

  return { success: true };
});

ipcMain.handle('stop-mining', async () => {
  if (minerProcess) {
    minerProcess.kill();
    minerProcess = null;
  }
  return { success: true };
});

ipcMain.handle('get-miner-balance', async (event, { nodeUrl, address }) => {
  const response = await fetch(`${nodeUrl}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      jsonrpc: '2.0',
      method: 'getbalance',
      params: [address],
      id: 1
    })
  });
  const data = await response.json();
  return data.result;
});

// --- Wallet IPC handlers (relay to wallet-backend) ---

const walletMethods = [
  'wallet_exists', 'create_wallet', 'import_wallet',
  'unlock', 'lock', 'get_mnemonic', 'derive_address',
  'list_addresses', 'get_balance', 'send', 'get_history',
  'get_private_key', 'import_private_key', 'set_node'
];

walletMethods.forEach(method => {
  ipcMain.handle('wallet-' + method, async (_, params) => {
    try {
      return await callWalletBackend(method, params || {});
    } catch (e) {
      return { error: e.message };
    }
  });
});
