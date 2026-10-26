// XYZ 新页面公共函数
const api = async (method, url, body) => {
  const res = await fetch(url, {
    method, headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined, credentials: 'same-origin',
  });
  if (res.status === 401 && !url.startsWith('/api/v1/auth/')) {
    location.href = '/login?next=' + encodeURIComponent(location.pathname + location.search);
    throw new Error('请先登录');
  }
  let data = null;
  try { data = await res.json(); } catch (e) {}
  if (!res.ok || (data && data.error)) throw new Error((data && data.error) || ('请求失败 ' + res.status));
  return data;
};
const $ = (s, el = document) => el.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
function toast(msg, err) {
  let t = $('#toast');
  if (!t) { t = document.createElement('div'); t.id = 'toast'; t.className = 'toast'; document.body.appendChild(t); }
  t.textContent = msg; t.className = 'toast show' + (err ? ' err' : '');
  clearTimeout(t._h); t._h = setTimeout(() => t.className = 'toast', 2200);
}
// 弹窗表单：fields=[{name,label,type,value,options:[[v,t]],placeholder}]，返回 Promise<值对象|null>
function formModal(title, fields, okText = '保存') {
  return new Promise(resolve => {
    const bg = document.createElement('div'); bg.className = 'modal-bg';
    const inputs = fields.map(f => {
      const id = 'f_' + f.name, v = esc(f.value ?? '');
      let ctl;
      if (f.type === 'select') ctl = `<select id="${id}">${f.options.map(([ov, ot]) => `<option value="${esc(ov)}" ${String(ov) === String(f.value ?? '') ? 'selected' : ''}>${esc(ot)}</option>`).join('')}</select>`;
      else if (f.type === 'textarea') ctl = `<textarea id="${id}" rows="${f.rows || 8}" placeholder="${esc(f.placeholder || '')}">${v}</textarea>`;
      else if (f.type === 'checkbox') ctl = `<label style="display:flex;gap:6px;align-items:center;color:var(--t1)"><input id="${id}" type="checkbox" style="width:auto" ${f.value ? 'checked' : ''}>${esc(f.text || '')}</label>`;
      else ctl = `<input id="${id}" type="${f.type || 'text'}" value="${v}" placeholder="${esc(f.placeholder || '')}" autocomplete="off">`;
      return `<div class="field"><label>${esc(f.label)}</label>${ctl}${f.hint ? `<div class="muted" style="font-size:12px;margin-top:4px">${esc(f.hint)}</div>` : ''}</div>`;
    }).join('');
    bg.innerHTML = `<div class="modal"><h3>${esc(title)}</h3>${inputs}<div class="row" style="justify-content:flex-end;margin-top:6px"><button class="btn" data-x>取消</button><button class="btn primary" data-ok>${esc(okText)}</button></div></div>`;
    document.body.appendChild(bg);
    const close = v => { bg.remove(); resolve(v); };
    bg.querySelector('[data-x]').onclick = () => close(null);
    bg.onclick = e => { if (e.target === bg) close(null); };
    bg.querySelector('[data-ok]').onclick = () => {
      const out = {};
      fields.forEach(f => { const el = bg.querySelector('#f_' + f.name); out[f.name] = f.type === 'checkbox' ? el.checked : el.value; });
      close(out);
    };
    const first = bg.querySelector('input,select,textarea'); if (first) first.focus();
    bg.addEventListener('keydown', e => { if (e.key === 'Enter' && e.target.tagName === 'INPUT') bg.querySelector('[data-ok]').click(); if (e.key === 'Escape') close(null); });
  });
}
