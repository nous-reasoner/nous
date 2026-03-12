const { contextBridge, ipcRenderer } = require('electron');

// Only expose specific, validated IPC methods to the renderer
contextBridge.exposeInMainWorld('api', {
  invoke: (channel, ...args) => {
    const allowed = [
      'start-mining', 'stop-mining', 'get-miner-balance',
      'wallet-wallet_exists', 'wallet-create_wallet', 'wallet-import_wallet',
      'wallet-unlock', 'wallet-lock', 'wallet-get_mnemonic', 'wallet-derive_address',
      'wallet-list_addresses', 'wallet-get_balance', 'wallet-send', 'wallet-get_history',
      'wallet-get_private_key', 'wallet-import_private_key', 'wallet-set_node'
    ];
    if (!allowed.includes(channel)) {
      return Promise.reject(new Error('IPC channel not allowed: ' + channel));
    }
    return ipcRenderer.invoke(channel, ...args);
  },
  on: (channel, callback) => {
    const allowed = ['miner-log', 'miner-stopped'];
    if (!allowed.includes(channel)) return;
    ipcRenderer.on(channel, (event, ...args) => callback(...args));
  },
  removeAllListeners: (channel) => {
    ipcRenderer.removeAllListeners(channel);
  }
});
