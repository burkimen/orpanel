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
    location.reload();
}

async function resetSettings() {
    if (!confirm(T.SettingResetConfirm)) return;
    await fetch('/api/settings/reset', { method: 'POST' });
    location.reload();
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
    info.textContent = T.SettingUpdating;

    try {
        const res = await fetch('/api/update/check');
        const data = await res.json();
        if (!data.updateAvailable) {
            info.textContent = T.SettingUpToDate + ' (v' + data.currentVersion + ')';
            btn.disabled = false;
            return;
        }
        info.textContent = T.SettingUpdateAvail + ': v' + data.currentVersion + ' → v' + data.latestVersion;
        btn.innerHTML = '<span class="material-symbols-rounded">system_update</span> ' + T.SettingUpdateNow;
        btn.disabled = false;
        btn.onclick = async function() {
            btn.disabled = true;
            info.textContent = T.SettingUpdating;
            await fetch('/api/update/install', { method: 'POST' });
            info.textContent = T.SettingUpdateDone;
            setTimeout(() => location.reload(), 2000);
        };
    } catch(e) {
        info.textContent = T.SettingUpdateFailed;
        btn.disabled = false;
    }
}

checkAutoStart();
loadSettings();
loadLogRetention();
