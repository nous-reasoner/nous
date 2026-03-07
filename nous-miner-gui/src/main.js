const { app, BrowserWindow, ipcMain } = require('electron');
const path = require('path');
const { spawn } = require('child_process');

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
