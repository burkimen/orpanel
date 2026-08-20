function switchTab(tabId, btn) {
    document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
    document.querySelectorAll('.tab-content').forEach(el => {
        if (el.id === tabId) el.classList.add('active');
    });
    if (btn) {
        document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
        btn.classList.add('active');
    }
    if (tabId === 'terminal-tab') setTimeout(() => fitAddon.fit(), 50);
    else if (tabId === 'logs-tab') fetchFileLogs();
}

const term = new Terminal({
    theme: { background: '#0b0d10', foreground: '#e2e2e6' },
    fontFamily: 'Consolas, "Courier New", monospace',
    fontSize: 13, cursorBlink: true, convertEol: true
});
const fitAddon = new FitAddon.FitAddon();
term.loadAddon(fitAddon);
term.open(document.getElementById('terminal'));
setTimeout(() => fitAddon.fit(), 100);
window.addEventListener('resize', () => fitAddon.fit());

const statusBadge = document.getElementById('statusBadge');
const statusText = document.getElementById('statusText');
const btnStart = document.getElementById('btnStart');
const btnStop = document.getElementById('btnStop');
const btnRestart = document.getElementById('btnRestart');
const btnOpenOmni = document.getElementById('btnOpenOmni');
const serverLogsBox = document.getElementById('server-logs');

function updateUI(isRunning) {
    const h = window.lastHealth;
    const block = h && (!h.installed || h.opRunning || h.status === 'installing' || h.status === 'not_installed' || h.health === 'installing');
    if (block) {
        statusText.textContent = h ? h.message : T.StatusClosed;
        statusBadge.classList.remove("active");
        statusBadge.querySelector('.material-symbols-rounded').textContent = "cloud_off";
        btnStart.disabled = true; btnStop.disabled = true; btnRestart.disabled = true; btnOpenOmni.disabled = true;
        return;
    }
    if (isRunning) {
        statusText.textContent = T.StatusActive;
        statusBadge.classList.add("active");
        statusBadge.querySelector('.material-symbols-rounded').textContent = "radio_button_checked";
        btnStart.disabled = true; btnStop.disabled = false; btnRestart.disabled = false; btnOpenOmni.disabled = false;
    } else {
        statusText.textContent = T.StatusClosed;
        statusBadge.classList.remove("active");
        statusBadge.querySelector('.material-symbols-rounded').textContent = "radio_button_unchecked";
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
        serverLogsBox.textContent = text;
        serverLogsBox.scrollTop = serverLogsBox.scrollHeight;
    } catch(e) {}
}

checkStatus();
setInterval(checkStatus, 3000);
setInterval(fetchLogs, 1000);
if (document.getElementById('logs-tab').classList.contains('active')) setInterval(fetchFileLogs, 3000);
term.write('\x1b[38;2;181;204;140m' + T.TerminalReady + '\x1b[0m\r\n\r\n');
