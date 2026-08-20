function toggleSidebar() {
    const sb = document.getElementById('sidebar');
    if (!sb) return;
    sb.classList.toggle('collapsed');
    localStorage.setItem('orpanel:sidebar', sb.classList.contains('collapsed') ? 'collapsed' : '');
    setTimeout(() => fitAddon.fit(), 200);
}

function cycleTheme() {
    const order = ['light','dark','system'];
    const idx = order.indexOf(currentTheme);
    const next = order[(idx+1)%order.length];
    setTheme(next);
    const icon = document.getElementById('themeIcon');
    if (icon) {
        if (next === 'light') icon.textContent = 'light_mode';
        else if (next === 'dark') icon.textContent = 'dark_mode';
        else icon.textContent = 'brightness_auto';
    }
}

function syncTopbar(tabId) {
    const mapTitle = {
        'terminal-tab': T.TabTerminal,
        'logs-tab': T.TabLogs,
        'settings-tab': T.TabSettings
    };
    const mapSub = {
        'terminal-tab': T.TopbarSubtitle,
        'logs-tab': T.TabLogs,
        'settings-tab': T.TabSettings
    };
    const t = document.getElementById('topbarTitle');
    const s = document.getElementById('topbarSubtitle');
    if (t) t.textContent = mapTitle[tabId] || T.TabTerminal;
    if (s) s.textContent = mapSub[tabId] || '';
}

// restore sidebar state
try {
    if (localStorage.getItem('orpanel:sidebar')==='collapsed') {
        document.getElementById('sidebar')?.classList.add('collapsed');
    }
} catch(e) {}

// wrap switchTab to sync sidebar nav and topbar
const _origSwitchTab = window.switchTab;
window.switchTab = function(tabId, btn) {
    _origSwitchTab(tabId, btn);
    document.querySelectorAll('.nav-item').forEach(n=>{
        n.classList.toggle('active', n.dataset.tab===tabId);
    });
    syncTopbar(tabId);
};

// init topbar
syncTopbar('terminal-tab');
