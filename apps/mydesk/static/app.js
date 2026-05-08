const App = {
  pages: {},

  registerPage(name, render) {
    this.pages[name] = render;
  },

  init() {
    window.addEventListener('hashchange', () => this.route());
    this.route();
    this.loadMeta();
  },

  async loadMeta() {
    try {
      const data = await Api.get('/api/status');
      document.getElementById('nav-version').textContent = 'v' + data.version;
      const el = document.getElementById('nav-config-dir');
      el.textContent = data.configDir;
      el.title = data.configDir;
    } catch (e) {
      document.getElementById('nav-version').textContent = 'error';
      document.getElementById('nav-config-dir').textContent = 'Connection failed';
      toast('Failed to load status: ' + e.message, 'error');
    }
  },

  route() {
    const hash = location.hash || '#/';
    const path = hash.replace('#', '');
    let page = 'dashboard';
    if (path === '/links') page = 'links';
    else if (path === '/config') page = 'config';
    else if (path === '/provision') page = 'provision';

    // Update nav
    document.querySelectorAll('.nav-link').forEach(el => {
      el.classList.toggle('active', el.dataset.page === page);
    });

    // Update title
    const titles = { dashboard: 'Dashboard', links: 'Links', config: 'Config', provision: 'Provision' };
    document.getElementById('page-title').textContent = titles[page] || '';

    // Render page
    const content = document.getElementById('content');
    content.innerHTML = '<p class="text-gray-400">Loading...</p>';
    if (this.pages[page]) {
      this.pages[page](content);
    }
  },

  isDryRun() {
    return document.getElementById('global-dry-run').checked;
  }
};

const Api = {
  async get(url) {
    const res = await fetch(url);
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data;
  },

  async post(url, body) {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data;
  },

  async put(url, body) {
    const res = await fetch(url, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data;
  }
};

function toast(message, type = 'info') {
  const container = document.getElementById('toast-container');
  const el = document.createElement('div');
  el.className = `toast toast-${type}`;
  el.textContent = message;
  container.appendChild(el);
  setTimeout(() => el.remove(), 4000);
}

function esc(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

function h(tag, attrs, ...children) {
  const el = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (k === 'className') el.className = v;
      else if (k === 'onclick') el.onclick = v;
      else if (k === 'innerHTML') el.innerHTML = v;
      else el.setAttribute(k, v);
    }
  }
  for (const child of children) {
    if (typeof child === 'string') el.appendChild(document.createTextNode(child));
    else if (child) el.appendChild(child);
  }
  return el;
}
