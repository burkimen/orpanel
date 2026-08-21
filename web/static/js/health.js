const healthCard = document.getElementById('omniHealthCard');
const healthBadge = document.getElementById('healthBadge');
const healthIcon = document.getElementById('healthIcon');
const healthIconWrap = document.getElementById('healthIconWrap');
const healthValVersion = document.getElementById('healthValVersion');
const healthValNode = document.getElementById('healthValNode');
const healthValPath = document.getElementById('healthValPath');
const healthValPort = document.getElementById('healthValPort');
const healthValUpdate = document.getElementById('healthValUpdate');
const healthActions = document.getElementById('healthActions');
const healthProgress = document.getElementById('healthProgress');
const healthEmpty = document.getElementById('healthEmpty');

async function loadOmniHealth() {
    try {
        const res = await fetch('/api/omni/health');
        const h = await res.json();
        renderHealth(h);
    } catch(e) {
        if (healthCard) healthCard.style.display='none';
        if (healthEmpty) { healthEmpty.style.display='flex'; healthEmpty.querySelector('#healthEmptyText').textContent = T.HealthLoading; }
    }
}

function renderHealth(h) {
    if (!healthCard || !h) return;
    window.lastHealth = h;
    healthCard.style.display = 'flex';
    if (healthEmpty) healthEmpty.style.display='none';

    // badge + header icon wrap color
    let badgeText = h.status, badgeCls = "error", icon = "hub";
    let wrapTone = "error";
    if (h.status === "running") { badgeText = T.HealthBadgeRunning; badgeCls = "ok"; icon = "check_circle"; wrapTone = "ok"; }
    else if (h.status === "stopped") { badgeText = T.HealthBadgeStopped; badgeCls = "warn"; icon = "pause_circle"; wrapTone = "warn"; }
    else if (h.status === "not_installed") { badgeText = T.HealthBadgeNotInstalled; badgeCls = "error"; icon = "cloud_off"; wrapTone = "error"; }
    else if (h.status === "port_conflict") { badgeText = T.HealthBadgePortConflict; badgeCls = "error"; icon = "error"; wrapTone = "error"; }
    else if (h.status === "corrupt") { badgeText = T.HealthBadgeCorrupt; badgeCls = "error"; icon = "broken_image"; wrapTone = "error"; }
    else if (h.status === "installing") { badgeText = T.HealthBadgeInstalling; badgeCls = "warn"; icon = "hourglass_top"; wrapTone = "warn"; }
    healthBadge.textContent = badgeText;
    healthBadge.className = "health-badge " + badgeCls;
    healthIcon.textContent = icon;
    if (healthIconWrap) healthIconWrap.className = "health-icon-wrap tone-" + wrapTone;

    // rows
    const rowVer = document.getElementById('healthRowVersion');
    if (rowVer) {
        if (h.installed && h.version) {
            healthValVersion.innerHTML = h.version;
            rowVer.style.display = 'flex';
        } else if (!h.installed) {
            healthValVersion.textContent = T.HealthBadgeNotInstalled;
            rowVer.style.display = 'flex';
        } else {
            rowVer.style.display = 'none';
        }
    }

    if (!h.nodeOk) healthValNode.innerHTML = (h.nodeVersion || T.HealthNodeNotFound) + " <span class='health-tag error'>" + T.HealthNodeReq + "</span>";
    else healthValNode.textContent = h.nodeVersion || "—";

    healthValPath.textContent = h.path || "—";
    const portStatus = h.portFree ? T.HealthPortFreeShort : T.HealthPortOccupiedShort;
    const portHealth = h.health || "";
    healthValPort.textContent = portStatus + " · " + portHealth;

    // Update row: show only when installed and update available
    const rowUpdate = document.getElementById('healthRowUpdate');
    if (rowUpdate) {
        if (h.installed && h.updateAvailable && h.latest) {
            healthValUpdate.innerHTML = "<strong>" + h.version + "</strong> → <strong>" + h.latest + "</strong>";
            rowUpdate.style.display = 'flex';
        } else {
            rowUpdate.style.display = 'none';
        }
    }

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
