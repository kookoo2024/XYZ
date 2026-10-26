// 注入到旧页面：右下角显示当前账号，可进管理后台/退出；会话过期时自动跳登录页。
(function () {
  const origFetch = window.fetch;
  window.fetch = async function (...args) {
    const res = await origFetch.apply(this, args);
    if (res.status === 401) {
      const u = typeof args[0] === 'string' ? args[0] : (args[0] && args[0].url) || '';
      if (u.indexOf('/api/v1/auth/') < 0) location.href = '/login?next=' + encodeURIComponent(location.pathname + location.search);
    }
    return res;
  };
  origFetch('/api/v1/auth/me', { credentials: 'same-origin' }).then(r => r.ok ? r.json() : null).then(u => {
    if (!u) return;
    const st = document.createElement('style');
    st.textContent = `#xyz-ub{position:fixed;right:14px;bottom:14px;z-index:99999;font:13px system-ui,"Microsoft YaHei",sans-serif}
#xyz-ub .av{width:34px;height:34px;border-radius:50%;background:#7c6aff;color:#fff;display:flex;align-items:center;justify-content:center;cursor:pointer;box-shadow:0 4px 14px rgba(124,106,255,.35);opacity:.55;transition:.2s;font-weight:600}
#xyz-ub:hover .av{opacity:1}
#xyz-ub .menu{display:none;position:absolute;right:0;bottom:42px;background:#fff;color:#111;border-radius:12px;box-shadow:0 10px 40px rgba(0,0,0,.18);padding:6px;min-width:170px}
#xyz-ub.open .menu{display:block}
#xyz-ub .who{padding:8px 10px;color:#6b7280;border-bottom:1px solid #eee;margin-bottom:4px}
#xyz-ub a{display:block;padding:8px 10px;border-radius:8px;color:#111;text-decoration:none;cursor:pointer}
#xyz-ub a:hover{background:#f3f4f6}
:fullscreen #xyz-ub,body.presenting #xyz-ub{display:none}`;
    document.head.appendChild(st);
    const box = document.createElement('div'); box.id = 'xyz-ub';
    const roleName = { admin: '管理员', teacher: '教师', student: '学生' }[u.role] || u.role;
    box.innerHTML = `<div class="menu"><div class="who">${u.name}（${roleName}）${u.local ? '<br><small>服务器本机，免登录</small>' : ''}</div>
      <a href="/admin">⚙️ 管理后台</a>${u.local ? '<a href="/login">🔑 登录个人账号</a>' : '<a data-out>🚪 退出登录</a>'}</div>
      <div class="av" title="${u.name}">${(u.name || '?').slice(-1)}</div>`;
    document.body.appendChild(box);
    box.querySelector('.av').onclick = () => box.classList.toggle('open');
    document.addEventListener('click', e => { if (!box.contains(e.target)) box.classList.remove('open'); });
    const out = box.querySelector('[data-out]');
    if (out) out.onclick = () => origFetch('/api/v1/auth/logout', { method: 'POST' }).then(() => location.href = '/login');
    document.addEventListener('fullscreenchange', () => { box.style.display = document.fullscreenElement ? 'none' : ''; });
  }).catch(() => {});
})();
