const { app, BrowserWindow, ipcMain, dialog } = require('electron');
const path = require('path');
const { spawn } = require('child_process');
const crypto = require('crypto');
const fs = require('fs');
const os = require('os');

const WALLET_DIR = path.join(os.homedir(), '.nous-miner');
const WALLET_PATH = path.join(WALLET_DIR, 'wallet.json');

let mainWindow;
let minerProcess;

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 800,
    height: 700,
    webPreferences: {
      nodeIntegration: true,
      contextIsolation: false
    }
  });

  mainWindow.loadFile('public/index.html');
}

app.whenReady().then(createWindow);

app.on('window-all-closed', () => {
  if (minerProcess) minerProcess.kill();
  if (process.platform !== 'darwin') app.quit();
});

// IPC handlers
ipcMain.handle('start-mining', async (event, config) => {
  if (minerProcess) return { error: 'Already mining' };

  const minerPath = path.join(__dirname, '../backend/miner');
  const args = [
    '--node', config.nodeUrl,
    '--address', config.address,
    '--solver', config.solver || 'probsat'
  ];

  // AI config (for ai-guided and pure-ai)
  if (config.solver === 'ai-guided' || config.solver === 'pure-ai') {
    if (config.aiProvider) args.push('--ai-provider', config.aiProvider);
    if (config.apiKey) args.push('--api-key', config.apiKey);
    if (config.model) args.push('--model', config.model);
    if (config.baseUrl) args.push('--base-url', config.baseUrl);
  }

  // Custom solver config
  if (config.solver === 'custom' && config.scriptPath) {
    args.push('--script', config.scriptPath);
  }

  minerProcess = spawn(minerPath, args);

  minerProcess.stdout.on('data', (data) => {
    const log = data.toString();
    console.log('[Miner]', log);
    mainWindow.webContents.send('miner-log', log);
  });

  minerProcess.stderr.on('data', (data) => {
    const log = data.toString();
    console.error('[Miner Error]', log);
    mainWindow.webContents.send('miner-log', log);
  });

  minerProcess.on('error', (err) => {
    console.error('[Miner Process Error]', err);
    mainWindow.webContents.send('miner-log', '[FATAL] ' + err.message);
    minerProcess = null;
  });

  minerProcess.on('close', () => {
    minerProcess = null;
    mainWindow.webContents.send('miner-stopped');
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

ipcMain.handle('get-balance', async (event, { nodeUrl, address }) => {
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

// --- Wallet management ---

function hash160(buf) {
  const sha = crypto.createHash('sha256').update(buf).digest();
  return crypto.createHash('ripemd160').update(sha).digest();
}

// Bech32m encoding
const BECH32M_CONST = 0x2bc830a3;
const CHARSET = 'qpzry9x8gf2tvdw0s3jn54khce6mua7l';

function bech32mPolymod(values) {
  const GEN = [0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3];
  let chk = 1;
  for (const v of values) {
    const b = chk >> 25;
    chk = ((chk & 0x1ffffff) << 5) ^ v;
    for (let i = 0; i < 5; i++) if ((b >> i) & 1) chk ^= GEN[i];
  }
  return chk;
}

function bech32mHrpExpand(hrp) {
  const ret = [];
  for (let i = 0; i < hrp.length; i++) ret.push(hrp.charCodeAt(i) >> 5);
  ret.push(0);
  for (let i = 0; i < hrp.length; i++) ret.push(hrp.charCodeAt(i) & 31);
  return ret;
}

function bech32mEncode(hrp, data5) {
  const values = bech32mHrpExpand(hrp).concat(data5);
  const polymod = bech32mPolymod(values.concat([0, 0, 0, 0, 0, 0])) ^ BECH32M_CONST;
  const checksum = [];
  for (let i = 0; i < 6; i++) checksum.push((polymod >> (5 * (5 - i))) & 31);
  let result = hrp + '1';
  for (const d of data5.concat(checksum)) result += CHARSET[d];
  return result;
}

function convertBits(data, fromBits, toBits, pad) {
  let acc = 0, bits = 0;
  const ret = [];
  const maxv = (1 << toBits) - 1;
  for (const d of data) {
    acc = (acc << fromBits) | d;
    bits += fromBits;
    while (bits >= toBits) {
      bits -= toBits;
      ret.push((acc >> bits) & maxv);
    }
  }
  if (pad && bits > 0) ret.push((acc << (toBits - bits)) & maxv);
  return ret;
}

function pubKeyHashToAddress(hash160Buf) {
  const data5 = convertBits(Array.from(hash160Buf), 8, 5, true);
  return bech32mEncode('nous', [0].concat(data5)); // witness version 0
}

function generateWallet() {
  // Generate 32 random bytes as private key
  const privKeyBuf = crypto.randomBytes(32);
  const privKeyHex = privKeyBuf.toString('hex');

  // Use tiny-secp256k1 to derive public key
  const secp256k1 = require('tiny-secp256k1');
  const pubKeyCompressed = Buffer.from(secp256k1.pointFromScalar(privKeyBuf, true));

  const pkh = hash160(pubKeyCompressed);
  const address = pubKeyHashToAddress(pkh);

  return {
    private_key: privKeyHex,
    public_key: pubKeyCompressed.toString('hex'),
    address: address
  };
}

ipcMain.handle('create-wallet', async () => {
  const wallet = generateWallet();
  fs.mkdirSync(WALLET_DIR, { recursive: true });
  fs.writeFileSync(WALLET_PATH, JSON.stringify(wallet, null, 2));
  return wallet;
});

ipcMain.handle('load-wallet', async () => {
  if (!fs.existsSync(WALLET_PATH)) return null;
  return JSON.parse(fs.readFileSync(WALLET_PATH, 'utf8'));
});

ipcMain.handle('export-private-key', async () => {
  if (!fs.existsSync(WALLET_PATH)) throw new Error('No wallet found');
  const wallet = JSON.parse(fs.readFileSync(WALLET_PATH, 'utf8'));
  const { canceled, filePath } = await dialog.showSaveDialog(mainWindow, {
    title: 'Export Wallet Backup',
    defaultPath: path.join(os.homedir(), 'Desktop', 'nous-wallet-backup.json'),
    filters: [{ name: 'JSON', extensions: ['json'] }]
  });
  if (canceled || !filePath) return { canceled: true };
  fs.writeFileSync(filePath, JSON.stringify(wallet, null, 2));
  return { path: filePath, address: wallet.address };
});
