function toggleSidebar() {
    const app = document.querySelector('.app-layout');
    const icon = document.getElementById('sidebarIcon');
    if (!app) return;
    app.classList.toggle('sidebar-collapsed');
    const collapsed = app.classList.contains('sidebar-collapsed');
    if (icon) icon.textContent = collapsed ? 'chevron_right' : 'chevron_left';
    localStorage.setItem('orpanel:sidebar', collapsed ? 'collapsed' : '');
    setTimeout(() => fitAddon.fit(), 200);
}

// restore sidebar state
try {
    if (localStorage.getItem('orpanel:sidebar')==='collapsed') {
        document.querySelector('.app-layout')?.classList.add('sidebar-collapsed');
        const icon = document.getElementById('sidebarIcon');
        if (icon) icon.textContent = 'chevron_right';
    }
} catch(e) {}
