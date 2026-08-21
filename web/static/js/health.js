const healthCard = document.getElementById('omniHealthCard');
const healthBadge = document.getElementById('healthBadge');
const healthBody = document.getElementById('healthBody');
const healthMeta = document.getElementById('healthMeta');
const healthActions = document.getElementById('healthActions');
const healthIcon = document.getElementById('healthIcon');
const healthProgress = document.getElementById('healthProgress');

async function loadOmniHealth() {
    try {
        const res = await fetch('/api/omni/health');
        const h = await res.json();
        renderHealth(h);
    } catch(e) {
        if (healthCard) healthCard.style.display='none';
    }
}

function renderHealth(h) {
    if (!healthCard || !h) return;
    window.lastHealth = h;
    healthCard.style.display = 'flex';
    let badgeText = h.status, badgeCls = "error", icon = "warning";
    if (h.status === "running") { badgeText = T.HealthBadgeRunning; badgeCls = "ok"; icon = "check_circle"; }
    else if (h.status === "stopped") { badgeText = T.HealthBadgeStopped; badgeCls = "warn"; icon = "pause_circle"; }
    else if (h.status === "not_installed") { badgeText = T.HealthBadgeNotInstalled; badgeCls = "error"; icon = "cloud_off"; }
    else if (h.status === "port_conflict") { badgeText = T.HealthBadgePortConflict; badgeCls = "error"; icon = "error"; }
    else if (h.status === "corrupt") { badgeText = T.HealthBadgeCorrupt; badgeCls = "error"; icon = "broken_image"; }
    else if (h.status === "installing") { badgeText = T.HealthBadgeInstalling; badgeCls = "warn"; icon = "hourglass_top"; }
    healthBadge.textContent = badgeText;
    healthBadge.className = "health-badge " + badgeCls;
    healthIcon.textContent = icon;
    let body = h.message || "";
    if (h.installed && h.version) {
        body += "<br><small>" + T.HealthInstalled + ":</small> <strong>" + h.version + "</strong> <small>(" + h.path + ")</small>";
        if (h.latest) body += " → <strong>" + h.latest + "</strong>";
        if (h.updateAvailable) body += " <span style='color:var(--color-warning)'>" + T.HealthUpdateAvail + "</span>";
    }
    if (!h.nodeOk) body += "<br>Node: " + (h.nodeVersion || T.HealthNodeNotFound) + " (<strong>" + T.HealthNodeReq + "</strong>)";
    else if (h.nodeVersion) body += "<br>Node " + h.nodeVersion;
    healthBody.innerHTML = body;
    let meta = "";
    if (h.installed) meta += '<span><span class="material-symbols-rounded" style="font-size:14px">folder</span>' + h.path + '</span>';
    meta += '<span><span class="material-symbols-rounded" style="font-size:14px">lan</span>:' + (h.portFree ? T.HealthPortFree : T.HealthPortOccupied) + ' 20128</span>';
    if (h.health) meta += '<span>health: ' + h.health + '</span>';
    healthMeta.innerHTML = meta;
    const installing = h.opRunning || h.status === 'installing';
    if (healthProgress) healthProgress.style.display = installing ? 'block' : 'none';
    let acts = "";
    const busy = healthActions.dataset.busy === "1" || h.opRunning;
    if (!h.installed) {
        if (installing) acts += '<button class="btn-health primary" disabled><span class="material-symbols-rounded">hourglass_top</span> ' + T.HealthBtnInstalling + '</button>';
        else acts += '<button class="btn-health primary" onclick="doOmniAction(\'install\')"><span class="material-symbols-rounded">download</span> ' + T.HealthBtnInstall + '</button>';
    } else if (h.updateAvailable) {
        if (installing) acts += '<button class="btn-health warning" disabled><span class="material-symbols-rounded">hourglass_top</span> ' + T.HealthBtnInstalling + '</button>';
        else {
            acts += '<button class="btn-health warning" onclick="doOmniAction(\'update\')"><span class="material-symbols-rounded">system_update</span> ' + T.HealthBtnUpdate + '</button>';
            acts += '<button class="btn-health ghost" onclick="doOmniAction(\'reinstall\')"><span class="material-symbols-rounded">restart_alt</span> ' + T.HealthBtnReinstall + '</button>';
        }
    } else if (h.health==="port_conflict" || h.status==="corrupt") {
        if (installing) acts += '<button class="btn-health warning" disabled><span class="material-symbols-rounded">hourglass_top</span> ' + T.HealthBtnInstalling + '</button>';
        else {
            acts += '<button class="btn-health warning" onclick="doOmniAction(\'repair\')"><span class="material-symbols-rounded">build</span> ' + T.HealthBtnRepair + '</button>';
            acts += '<button class="btn-health ghost" onclick="doOmniAction(\'reinstall\')"><span class="material-symbols-rounded">restart_alt</span> ' + T.HealthBtnReinstall + '</button>';
        }
    } else if (h.status==="stopped") {
        if (installing) acts += '<button class="btn-health warning" disabled><span class="material-symbols-rounded">hourglass_top</span> ' + T.HealthBtnInstalling + '</button>';
        else acts += '<button class="btn-health ghost" onclick="doOmniAction(\'reinstall\')"><span class="material-symbols-rounded">restart_alt</span> ' + T.HealthBtnReinstall + '</button>';
    } else {
        if (installing) acts += '<button class="btn-health warning" disabled><span class="material-symbols-rounded">hourglass_top</span> ' + T.HealthBtnInstalling + '</button>';
        else acts += '<button class="btn-health ghost" onclick="doOmniAction(\'reinstall\')"><span class="material-symbols-rounded">restart_alt</span> ' + T.HealthBtnReinstall + '</button>';
    }
    healthActions.innerHTML = acts;
    const shouldDisableControls = !h.installed || h.opRunning || h.status === 'installing' || h.status === 'not_installed' || h.health === 'installing';
    [btnStart, btnStop, btnRestart, btnOpenOmni].forEach(btn => {
        if (shouldDisableControls) { btn.disabled = true; btn.setAttribute('title', h.message); }
        else { btn.removeAttribute('title'); }
    });
    if (!shouldDisableControls) checkStatus();
}

async function doOmniAction(action) {
    const btns = healthActions.querySelectorAll('button');
    btns.forEach(b=>b.disabled=true);
    healthActions.dataset.busy="1";
    if (healthProgress) healthProgress.style.display='block';
    if (healthProgress) { const bar = healthProgress.querySelector('.health-progress-bar'); if (bar) bar.style.width = '100%'; }
    const fastHealth = setInterval(loadOmniHealth, 2000);
    const fastLogs = setInterval(fetchLogs, 300);
    try {
        const res = await fetch('/api/omni/'+action, {method:'POST'});
        const j = await res.json().catch(()=>({}));
        if (!res.ok) term.write('\x1b[31m' + T.TermErr + ': ' + (j.error || res.statusText) +'\x1b[0m\r\n');
        else term.write('\x1b[32m' + T.TermStarted.replace('{action}', action) + '\x1b[0m\r\n');
    } catch(e) { term.write('\x1b[31m' + T.TermReqErr + ': '+e+'\x1b[0m\r\n'); }
    setTimeout(async()=>{ clearInterval(fastHealth); clearInterval(fastLogs); healthActions.dataset.busy="0"; if (healthProgress) healthProgress.style.display='none'; await loadOmniHealth(); }, 8000);
}

loadOmniHealth();
setInterval(loadOmniHealth, 8000);
