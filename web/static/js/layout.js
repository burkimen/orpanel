function toggleSidebar() {
    const sb = document.getElementById('sidebar');
    if (!sb) return;
    sb.classList.toggle('collapsed');
    localStorage.setItem('orpanel:sidebar', sb.classList.contains('collapsed') ? 'collapsed' : '');
}

function cycleTheme() {
    const order = ['light','dark','system'];
    const idx = order.indexOf(currentTheme);
    const next = order[(idx+1)%order.length];
    setTheme(next);
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
    const flag = document.getElementById('topbarFlag');
    const lang = document.getElementById('topbarLang');
    if (flag && lang) {
        const cur = localStorage.getItem('orpanel:lang') || LANG;
        let code='tr', f='tr';
        if(cur==='en'){ code='GB'; f='gb'; lang.textContent='EN'; }
        else if(cur==='es'){ code='ES'; f='es'; lang.textContent='ES'; }
        else lang.textContent='TR';
        flag.src='https://flagcdn.com/w40/'+f+'.png';
    }
}

// restore sidebar state
try {
    if (localStorage.getItem('orpanel:sidebar')==='collapsed') {
        document.getElementById('sidebar')?.classList.add('collapsed');
    }
} catch(e) {}

// wrap switchTab to also sync topbar and sidebar nav
const _origSwitchTab = window.switchTab;
window.switchTab = function(tabId, btn) {
    _origSwitchTab(tabId, btn);
    // sync sidebar nav active
    document.querySelectorAll('.nav-item').forEach(n=>{
        n.classList.toggle('active', n.dataset.tab===tabId);
    });
    syncTopbar(tabId);
    // also sync topbar lang
    try {
        const cur = localStorage.getItem('orpanel:lang') || LANG;
        localStorage.setItem('orpanel:lang', cur);
    } catch(e){}
};

// init topbar
syncTopbar('terminal-tab');
