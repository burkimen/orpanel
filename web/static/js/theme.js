let currentTheme = THEME;
const themeLink = document.getElementById('theme-variables');

function getEffectiveTheme(theme) {
    if (theme === 'system') {
        return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }
    return theme;
}

function applyTheme(theme) {
    currentTheme = theme;
    const effective = getEffectiveTheme(theme);
    if (themeLink) themeLink.href = '/themes/' + effective + '/variables.css';
    document.querySelectorAll('.theme-option').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.theme === theme);
    });
}

async function setTheme(theme) {
    applyTheme(theme);
    try {
        await fetch('/api/theme', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ theme })
        });
    } catch(e) {}
}

async function loadTheme() {
    try {
        const res = await fetch('/api/theme');
        const data = await res.json();
        if (data.theme && ['light','dark','system'].includes(data.theme)) applyTheme(data.theme);
        else applyTheme(currentTheme || 'system');
    } catch(e) { applyTheme(currentTheme || 'system'); }
}

try {
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
        if (currentTheme === 'system') applyTheme('system');
    });
} catch(e) {
    try { window.matchMedia('(prefers-color-scheme: dark)').addListener(() => { if(currentTheme==='system') applyTheme('system'); }); } catch(_){}
}

applyTheme(currentTheme || 'system');
