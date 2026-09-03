'use strict';
const { app, BrowserWindow, ipcMain, Tray, Menu, shell, screen } = require('electron');
const path = require('node:path');
const fs = require('node:fs');
const { collect, resolveClaudeDir, candidateDirs } = require('./parser.cjs');

const SETTINGS_FILE = () => path.join(app.getPath('userData'), 'settings.json');

function loadSettings() {
  try { return JSON.parse(fs.readFileSync(SETTINGS_FILE(), 'utf8')); } catch { return {}; }
}
function saveSettings(patch) {
  const next = { ...loadSettings(), ...patch };
  try {
    fs.mkdirSync(path.dirname(SETTINGS_FILE()), { recursive: true });
    fs.writeFileSync(SETTINGS_FILE(), JSON.stringify(next, null, 2));
  } catch { /* ignore */ }
  return next;
}

let win = null;
let tray = null;

function createWindow() {
  const settings = loadSettings();
  const display = screen.getPrimaryDisplay().workAreaSize;

  win = new BrowserWindow({
    width: 380,
    height: 620,
    x: settings.x ?? display.width - 400,
    y: settings.y ?? 40,
    frame: false,
    transparent: true,
    resizable: true,
    minWidth: 320,
    minHeight: 400,
    skipTaskbar: false,
    alwaysOnTop: settings.alwaysOnTop !== false,
    backgroundColor: '#00000000',
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  win.loadFile(path.join(__dirname, 'index.html'));
  win.on('moved', () => {
    const [x, y] = win.getPosition();
    saveSettings({ x, y });
  });
  win.on('closed', () => { win = null; });
}

function createTray() {
  const iconPath = path.join(__dirname, 'icon.png');
  if (!fs.existsSync(iconPath)) return;
  tray = new Tray(iconPath);
  tray.setToolTip('Claude Usage Widget');
  tray.setContextMenu(Menu.buildFromTemplate([
    { label: 'Show / Hide', click: () => (win && win.isVisible() ? win.hide() : win ? win.show() : createWindow()) },
    {
      label: 'Always on top',
      type: 'checkbox',
      checked: loadSettings().alwaysOnTop !== false,
      click: (item) => { saveSettings({ alwaysOnTop: item.checked }); win && win.setAlwaysOnTop(item.checked); },
    },
    { type: 'separator' },
    { label: 'Quit', click: () => app.quit() },
  ]));
  tray.on('double-click', () => win && win.show());
}

app.whenReady().then(() => {
  createWindow();
  createTray();
  app.on('activate', () => { if (BrowserWindow.getAllWindows().length === 0) createWindow(); });
});
app.on('window-all-closed', () => { if (process.platform !== 'darwin') app.quit(); });

// ── IPC ─────────────────────────────────────────────────────
ipcMain.handle('usage:get', async () => {
  const settings = loadSettings();
  const dir = resolveClaudeDir(settings.claudeDir);
  if (!dir) {
    return { error: 'No Claude logs found.', candidates: candidateDirs() };
  }
  try {
    const data = await collect(dir);
    return { ...data, budget: settings.budget ?? null };
  } catch (e) {
    return { error: String(e && e.message ? e.message : e), candidates: candidateDirs() };
  }
});

ipcMain.handle('settings:get', () => ({ ...loadSettings(), candidates: candidateDirs() }));
ipcMain.handle('settings:set', (_e, patch) => {
  const next = saveSettings(patch || {});
  if (win && typeof patch?.alwaysOnTop === 'boolean') win.setAlwaysOnTop(patch.alwaysOnTop);
  return next;
});

ipcMain.on('win:minimize', () => win && win.minimize());
ipcMain.on('win:close', () => win && win.hide());
ipcMain.on('win:open-docs', () => shell.openExternal('https://docs.anthropic.com/en/docs/claude-code'));
