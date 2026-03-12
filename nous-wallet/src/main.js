const { app, BrowserWindow, ipcMain } = require('electron');
const path = require('path');
const { spawn } = require('child_process');

let mainWindow;
let backend;
let requestId = 0;
const pending = new Map();

function startBackend() {
    const binaryName = process.platform === 'win32' ? 'wallet-backend.exe' : 'wallet-backend';
    const binaryPath = app.isPackaged
        ? path.join(process.resourcesPath, 'backend', binaryName)
        : path.join(__dirname, '../backend', binaryName);

    backend = spawn(binaryPath, [], { stdio: ['pipe', 'pipe', 'pipe'] });

    let buffer = '';
    backend.stdout.on('data', (data) => {
        buffer += data.toString();
        const lines = buffer.split('\n');
        buffer = lines.pop(); // keep incomplete line
        for (const line of lines) {
            if (!line.trim()) continue;
            try {
                const resp = JSON.parse(line);
                const resolve = pending.get(resp.id);
                if (resolve) {
                    pending.delete(resp.id);
                    resolve(resp);
                }
            } catch (e) {
                // ignore parse errors
            }
        }
    });

    backend.stderr.on('data', (data) => {
        console.log('[backend]', data.toString().trim());
    });

    backend.on('close', () => {
        console.log('backend exited');
        backend = null;
    });
}

function callBackend(method, params = {}) {
    return new Promise((resolve, reject) => {
        if (!backend) {
            reject(new Error('backend not running'));
            return;
        }
        const id = ++requestId;
        pending.set(id, resolve);
        const req = JSON.stringify({ method, params, id }) + '\n';
        backend.stdin.write(req);

        // Timeout after 30 seconds
        setTimeout(() => {
            if (pending.has(id)) {
                pending.delete(id);
                reject(new Error('timeout'));
            }
        }, 30000);
    });
}

function createWindow() {
    mainWindow = new BrowserWindow({
        width: 480,
        height: 720,
        titleBarStyle: 'hidden',
        trafficLightPosition: { x: 12, y: 12 },
        backgroundColor: '#0a0a0a',
        webPreferences: {
            nodeIntegration: true,
            contextIsolation: false
        }
    });
    mainWindow.loadFile('public/index.html');
}

app.whenReady().then(() => {
    startBackend();
    createWindow();
});

app.on('window-all-closed', () => {
    if (backend) backend.kill();
    app.quit();
});

// IPC handlers — relay to backend
const methods = [
    'wallet_exists', 'create_wallet', 'import_wallet',
    'unlock', 'lock', 'get_mnemonic', 'derive_address',
    'list_addresses', 'get_balance', 'send', 'get_history',
    'get_private_key', 'set_node'
];

methods.forEach(method => {
    ipcMain.handle(method, async (_, params) => {
        try {
            return await callBackend(method, params || {});
        } catch (e) {
            return { error: e.message };
        }
    });
});
