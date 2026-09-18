const autoStartToggle = document.getElementById('autoStartToggle');
const logRetentionSelect = document.getElementById('logRetention');

async function checkAutoStart() {
    const res = await fetch('/api/autostart');
    const data = await res.json();
    autoStartToggle.checked = data.isEnabled;
}

async function toggleAutoStart() {
    const isEnabled = autoStartToggle.checked;
    await fetch('/api/autostart', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ isEnabled })
    });
}

async function loadLogRetention() {
    try {
        const res = await fetch('/api/settings/log-retention');
        const data = await res.json();
        logRetentionSelect.value = data.hours;
    } catch(e) {}
}

async function setLogRetention() {
    const hours = parseInt(logRetentionSelect.value);
    await fetch('/api/settings/log-retention', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ hours })
    });
}

async function clearLogs() {
    if (!confirm(T.SettingResetConfirm)) return;
    await fetch('/api/settings/clear-logs', { method: 'POST' });
    showToast(T.SettingClearLogs, 'ok');
    setTimeout(() => location.reload(), 1000);
}

async function resetSettings() {
    if (!confirm(T.SettingResetConfirm)) return;
    await fetch('/api/settings/reset', { method: 'POST' });
    showToast(T.SettingReset, 'warn');
    setTimeout(() => location.reload(), 1000);
}

function toggleDropdown() {
    document.getElementById('langDropdown').classList.toggle('active');
    document.getElementById('dropdownOptions').classList.toggle('open');
}

window.addEventListener('click', function(e) {
    if (!document.getElementById('langDropdown').contains(e.target)) {
        document.getElementById('langDropdown').classList.remove('active');
        document.getElementById('dropdownOptions').classList.remove('open');
    }
});

async function loadSettings() {
    const res = await fetch('/api/language');
    const data = await res.json();
    if(data.language) setDropdownUI(data.language);
}

function setDropdownUI(lang) {
    let text = 'Türkçe', flag = 'tr';
    if(lang === 'en') { text = 'English'; flag = 'gb'; }
    if(lang === 'es') { text = 'Español'; flag = 'es'; }
    document.getElementById('selectedText').textContent = text;
    document.getElementById('selectedFlag').src = "https://flagcdn.com/w40/" + flag + ".png";
}

async function changeLanguage(lang, text, flag) {
    setDropdownUI(lang);
    document.getElementById('langDropdown').classList.remove('active');
    document.getElementById('dropdownOptions').classList.remove('open');
    await fetch('/api/language', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ language: lang })
    });
    location.reload();
}

async function checkAndUpdate() {
    const btn = document.getElementById('updateBtn');
    const info = document.getElementById('updateInfo');
    btn.disabled = true;
    showToast(T.SettingUpdating, 'info');

    try {
        const res = await fetch('/api/update/check');
        const data = await res.json();
        if (!data.updateAvailable) {
            info.textContent = T.SettingUpToDate + ' (v' + data.currentVersion + ')';
            btn.disabled = false;
            showToast(T.SettingUpToDate + ' v' + data.currentVersion, 'ok');
            return;
        }
        info.textContent = T.SettingUpdateAvail + ': v' + data.currentVersion + ' → v' + data.latestVersion;
        showToast(T.SettingUpdateAvail + ': v' + data.latestVersion + ' mevcut', 'warn');
        btn.innerHTML = '<span class="material-symbols-rounded">download</span> ' + T.SettingUpdateNow;
        btn.disabled = false;
        btn.onclick = async function() {
            btn.disabled = true;
            btn.innerHTML = '<span class="material-symbols-rounded">hourglass_top</span> ' + T.SettingUpdating;
            const startedToast = showToast(T.SettingUpdating, 'info', 60000);
            pollUpdateStatus(info, btn, data.currentVersion, startedToast);
            try {
                await fetch('/api/update/install', { method: 'POST' });
            } catch(e) {
                // The panel exits mid-update; loss of the response is expected.
            }
        };
    } catch(e) {
        info.textContent = T.SettingUpdateFailed;
        btn.disabled = false;
        showToast(T.SettingUpdateFailed, 'error');
    }
}

function updatePhaseLabel(s) {
    if (!s) return T.SettingUpdating;
    if (s.phase === 'downloading') return T.UpdatePhaseDownloading;
    if (s.phase === 'verifying') return T.UpdatePhaseVerifying;
    if (s.phase === 'applying') return T.UpdatePhaseApplying;
    if (s.phase === 'restarting') return T.UpdatePhaseRestarting;
    if (s.phase === 'failed') return T.UpdatePhaseFailed + (s.error ? ': ' + s.error : '');
    return T.SettingUpdating;
}

async function pollUpdateStatus(info, btn, prevVersion, startedToast) {
    const started = Date.now();
    let goneSince = 0;
    let lastPhase = '';
    const finish = (text, type) => {
        info.textContent = text;
        if (startedToast && startedToast.remove) startedToast.remove();
        showToast(text, type);
        btn.disabled = false;
    };
    const showProgress = (label) => {
        info.innerHTML = '<span class="material-symbols-rounded" style="vertical-align:-4px">hourglass_top</span> ' + label +
            '<span class="health-progress" style="display:block;margin-top:6px"><span class="health-progress-bar" style="width:100%"></span></span>';
    };
    showProgress(T.SettingUpdating);
    for (;;) {
        await new Promise(r => setTimeout(r, 650));
        let s = null;
        try {
            const res = await fetch('/api/update/status');
            s = await res.json();
        } catch(e) {
            if (!goneSince) goneSince = Date.now();
            showProgress(T.UpdateRestarting);
            if (Date.now() - goneSince > 60000 || Date.now() - started > 180000) {
                finish(T.SettingUpdateFailed + (lastPhase ? ' (' + lastPhase + ')' : '') + ' — ' + T.UpdateRefreshHint, 'error');
                return;
            }
            continue;
        }
        if (s.phase === 'idle' && s.currentVersion && prevVersion && s.currentVersion === prevVersion) {
            if (!goneSince) goneSince = Date.now();
            showProgress(T.UpdateRestarting);
            if (Date.now() - goneSince > 20000 || Date.now() - started > 180000) {
                finish(T.UpdateSameVersion, 'error');
                return;
            }
            continue;
        }
        if (s.currentVersion && prevVersion && s.currentVersion !== prevVersion) {
            finish(T.SettingUpdateDone, 'ok');
            location.reload();
            return;
        }
        const wasGone = goneSince > 0;
        goneSince = 0;
        if (s.phase === 'failed') {
            finish(updatePhaseLabel(s), 'error');
            return;
        }
        if (wasGone && s.currentVersion && prevVersion && s.currentVersion === prevVersion) {
            showProgress(T.UpdateRestarting);
            if (Date.now() - started > 180000) {
                finish(T.SettingUpdateFailed + ' (' + updatePhaseLabel(s) + ') — ' + T.UpdateRefreshHint, 'error');
                return;
            }
            continue;
        }
        lastPhase = updatePhaseLabel(s);
        showProgress(lastPhase);
        if (Date.now() - started > 180000) {
            finish(T.SettingUpdateFailed + ' (' + lastPhase + ') — ' + T.UpdateRefreshHint, 'error');
            return;
        }
    }
}

async function waitForPanelReturn(info, btn, started) {
    // Legacy helper, kept for compatibility; the version-derived completion
    // in pollUpdateStatus supersedes it.
    info.textContent = T.UpdateWaitReturn;
    const deadline = started + 60000;
    for (;;) {
        await new Promise(r => setTimeout(r, 750));
        try {
            const res = await fetch('/api/status');
            if (res.ok) {
                showToast(T.SettingUpdateDone, 'ok');
                location.reload();
                return;
            }
        } catch(e) {}
        if (Date.now() > deadline) {
            info.textContent = T.SettingUpdateFailed;
            showToast(T.SettingUpdateFailed, 'error');
            btn.disabled = false;
            return;
        }
        info.textContent = T.UpdateGone;
    }
}
