function switchTab(tabId, btn) {
    document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
    document.getElementById(tabId)?.classList.add('active');
    document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
    btn?.classList.add('active');
    document.getElementById('topbarTitle').textContent = btn?.dataset?.title || T.TabStatus;
    if (tabId === 'logs-tab') fetchFileLogs();
}

const term = new Terminal({
    theme: { background: '#0b0d10', foreground: '#e2e2e6' },
    fontFamily: 'Consolas, "Courier New", monospace',
    fontSize: 13, cursorBlink: true, convertEol: true
});
const fitAddon = new FitAddon.FitAddon();
term.loadAddon(fitAddon);
term.open(document.getElementById('terminal'));

const btnStart = document.getElementById('topBtnStart');
const btnStop = document.getElementById('topBtnStop');
const btnRestart = document.getElementById('topBtnRestart');
const btnOpenOmni = document.getElementById('topBtnOmni');
const serverLogsBox = document.getElementById('server-logs');

function updateUI(isRunning) {
    const h = window.lastHealth;
    const block = h && (!h.installed || h.opRunning || h.status === 'installing' || h.status === 'not_installed' || h.health === 'installing');
    if (block) {
        btnStart.disabled = true; btnStop.disabled = true; btnRestart.disabled = true; btnOpenOmni.disabled = true;
        return;
    }
    if (isRunning) {
        btnStart.disabled = true; btnStop.disabled = false; btnRestart.disabled = false; btnOpenOmni.disabled = false;
    } else {
        btnStart.disabled = false; btnStop.disabled = true; btnRestart.disabled = true; btnOpenOmni.disabled = true;
    }
}

async function sendCommand(cmd) {
    await fetch('/api/' + cmd, { method: 'POST' });
    checkStatus();
}

async function checkStatus() {
    const res = await fetch('/api/status');
    const data = await res.json();
    updateUI(data.isRunning);
}

let lastLogIndex = 0;
async function fetchLogs() {
    try {
        const res = await fetch('/api/logs?last=' + lastLogIndex);
        const data = await res.json();
        if (data.logs && data.logs.length > 0) {
            data.logs.forEach(line => term.write(line + '\r\n'));
            lastLogIndex = data.newIndex;
        }
    } catch(e) {}
}

async function fetchFileLogs() {
    try {
        const res = await fetch('/api/file-logs');
        const text = await res.text();
        if (serverLogsBox) { serverLogsBox.textContent = text; serverLogsBox.scrollTop = serverLogsBox.scrollHeight; }
    } catch(e) {}
}

// Terminal resize
const resizeHandle = document.getElementById('resizeHandle');
const terminalPanel = document.getElementById('terminalPanel');
if (resizeHandle && terminalPanel) {
    let startY, startH;
    resizeHandle.addEventListener('mousedown', e => {
        e.preventDefault();
        startY = e.clientY;
        startH = terminalPanel.offsetHeight;
        resizeHandle.classList.add('active');
        document.addEventListener('mousemove', onResize);
        document.addEventListener('mouseup', onResizeEnd);
    });
    function onResize(e) {
        const newH = startH - (e.clientY - startY);
        terminalPanel.style.height = Math.max(80, Math.min(newH, window.innerHeight * 0.6)) + 'px';
        fitAddon.fit();
    }
    function onResizeEnd() {
        resizeHandle.classList.remove('active');
        document.removeEventListener('mousemove', onResize);
        document.removeEventListener('mouseup', onResizeEnd);
    }
}

function toggleTerminalMax() {
    terminalPanel.classList.toggle('maximized');
    const icon = document.getElementById('terminalMaxIcon');
    if (icon) icon.textContent = terminalPanel.classList.contains('maximized') ? 'expand_more' : 'expand_less';
    setTimeout(() => fitAddon.fit(), 50);
}

// Init after DOM ready
window.addEventListener('load', () => {
    fitAddon.fit();
    checkStatus();
    loadOmniHealth();
});
setInterval(checkStatus, 3000);
setInterval(fetchLogs, 1000);
term.write('\x1b[38;2;181;204;140m' + T.TerminalReady + '\x1b[0m\r\n\r\n');
