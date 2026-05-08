App.registerPage('config', async (root) => {
  root.innerHTML = '';

  // Tabs
  const tabBar = h('div', { className: 'flex gap-0 border-b mb-4' });
  const tabLinks = h('button', { className: 'tab-btn active', onclick: () => switchTab('links-conf') }, 'links.conf');
  const tabNative = h('button', { className: 'tab-btn', onclick: () => switchTab('native-dirs') }, 'Native Dirs');
  tabBar.appendChild(tabLinks);
  tabBar.appendChild(tabNative);
  root.appendChild(tabBar);

  const contentArea = h('div', {});
  root.appendChild(contentArea);

  function switchTab(tab) {
    tabLinks.classList.toggle('active', tab === 'links-conf');
    tabNative.classList.toggle('active', tab === 'native-dirs');
    if (tab === 'links-conf') renderLinksConf();
    else renderNativeDirs();
  }

  async function renderLinksConf() {
    try {
      const data = await Api.get('/api/config/links-conf');
      contentArea.innerHTML = '';

      const card = h('div', { className: 'card' });

      const pathInfo = h('p', { className: 'text-xs text-gray-500 mb-2 font-mono' }, data.path);
      card.appendChild(pathInfo);

      const textarea = document.createElement('textarea');
      textarea.className = 'w-full h-64 font-mono text-sm border rounded p-3 bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-300';
      textarea.value = data.content;
      card.appendChild(textarea);

      const msgArea = h('div', { className: 'mt-2 text-sm' });
      card.appendChild(msgArea);

      const btnRow = h('div', { className: 'flex gap-2 mt-3' },
        h('button', { className: 'btn btn-secondary', onclick: async () => {
          try {
            const r = await Api.put('/api/config/links-conf', { content: textarea.value, dryRun: true });
            msgArea.innerHTML = `<p class="text-green-600">Valid: ${r.entriesParsed} entries parsed</p>`;
          } catch (e) {
            msgArea.innerHTML = `<p class="text-red-600">${esc(e.message)}</p>`;
          }
        }}, 'Validate'),
        h('button', { className: 'btn btn-primary', onclick: async () => {
          if (!confirm('Save links.conf?')) return;
          try {
            const r = await Api.put('/api/config/links-conf', { content: textarea.value, dryRun: false });
            toast(`Saved: ${r.entriesParsed} entries`, 'success');
            msgArea.innerHTML = '';
          } catch (e) {
            toast(e.message, 'error');
          }
        }}, 'Save')
      );
      card.appendChild(btnRow);

      contentArea.appendChild(card);
    } catch (e) {
      contentArea.innerHTML = `<p class="text-red-600">Error: ${esc(e.message)}</p>`;
    }
  }

  async function renderNativeDirs() {
    try {
      const data = await Api.get('/api/config/native-dirs');
      contentArea.innerHTML = '';

      for (const nd of data.dirs) {
        const card = h('div', { className: 'card mb-3' });
        const header = h('div', { className: 'flex items-center gap-2 mb-2' },
          h('span', { className: 'font-mono font-semibold' }, nd.dir + '/'),
          h('span', { className: 'text-sm text-gray-500' }, nd.targetBase ? `→ ${nd.targetBase}` : '(no target)'),
          h('span', { className: `badge ${nd.exists ? 'badge-ok' : 'badge-warn'}` },
            nd.exists ? `${nd.files.length} files` : 'missing')
        );
        card.appendChild(header);

        if (nd.files.length > 0) {
          const list = h('div', { className: 'grid grid-cols-2 md:grid-cols-3 gap-1' });
          for (const f of nd.files) {
            list.appendChild(h('span', { className: 'font-mono text-xs text-gray-600 bg-gray-50 px-2 py-1 rounded' }, f));
          }
          card.appendChild(list);
        }

        contentArea.appendChild(card);
      }
    } catch (e) {
      contentArea.innerHTML = `<p class="text-red-600">Error: ${esc(e.message)}</p>`;
    }
  }

  // Default: links.conf tab
  renderLinksConf();
});

// esc() is defined in app.js
