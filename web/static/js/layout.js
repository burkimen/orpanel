function toggleSidebar() {
    const sb = document.getElementById('sidebar');
    const app = document.querySelector('.app-layout');
    if (!sb) return;
    sb.classList.toggle('collapsed');
    if (app) app.classList.toggle('sidebar-collapsed', sb.classList.contains('collapsed'));
    localStorage.setItem('orpanel:sidebar', sb.classList.contains('collapsed') ? 'collapsed' : '');
    setTimeout(() => fitAddon.fit(), 200);
}

// restore sidebar state
try {
    if (localStorage.getItem('orpanel:sidebar')==='collapsed') {
        document.getElementById('sidebar')?.classList.add('collapsed');
        document.querySelector('.app-layout')?.classList.add('sidebar-collapsed');
    }
} catch(e) {}
