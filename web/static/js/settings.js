const autoStartToggle = document.getElementById('autoStartToggle');

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

checkAutoStart();
loadSettings();
