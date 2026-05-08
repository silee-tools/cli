App.registerPage('links', async (root) => {
  try {
    const data = await Api.get('/api/links');
    root.innerHTML = '';

    // Top bar
    const topBar = h('div', { className: 'flex items-center gap-3 mb-4 flex-wrap' },
      h('button', { className: 'btn btn-primary', onclick: async () => {
        try {
          const r = await Api.post('/api/links/link', { dryRun: App.isDryRun() });
          toast(`Linked: ${r.report.linked}, Skipped: ${r.report.skipped}, BackedUp: ${r.report.backedUp}, Failed: ${r.report.failed}`,
            r.report.failed > 0 ? 'error' : 'success');
          App.route();
        } catch (e) { toast(e.message, 'error'); }
      }}, 'Link All'),
      h('button', { className: 'btn btn-danger', onclick: async () => {
        if (!confirm('Unlink all symlinks and restore backups?')) return;
        try {
          const r = await Api.post('/api/links/unlink', { dryRun: App.isDryRun() });
          toast(`Unlinked: ${r.report.linked}, Skipped: ${r.report.skipped}, Failed: ${r.report.failed}`,
            r.report.failed > 0 ? 'error' : 'success');
          App.route();
        } catch (e) { toast(e.message, 'error'); }
      }}, 'Unlink All'),
      h('span', { className: 'text-sm text-gray-500 ml-auto' },
        `${data.summary.total} total, ${data.summary.linked} linked, ${data.summary.drifted} drifted`)
    );
    root.appendChild(topBar);

    // Filter
    const filterBar = h('div', { className: 'flex items-center gap-3 mb-4' });
    const filterSelect = h('select', { className: 'border rounded px-2 py-1 text-sm' });
    ['all', 'linked', 'drifted'].forEach(v => {
      const opt = h('option', { value: v }, v.charAt(0).toUpperCase() + v.slice(1));
      filterSelect.appendChild(opt);
    });
    const searchInput = h('input', {
      type: 'text', placeholder: 'Search source or target...',
      className: 'border rounded px-2 py-1 text-sm flex-1'
    });
    filterBar.appendChild(filterSelect);
    filterBar.appendChild(searchInput);
    root.appendChild(filterBar);

    // Table
    const table = h('div', { className: 'card overflow-x-auto' });
    const renderTable = () => {
      const filter = filterSelect.value;
      const search = searchInput.value.toLowerCase();

      let entries = data.entries;
      if (filter === 'linked') entries = entries.filter(e => e.status === 'linked');
      else if (filter === 'drifted') entries = entries.filter(e => e.status !== 'linked');
      if (search) entries = entries.filter(e =>
        e.source.toLowerCase().includes(search) || e.target.toLowerCase().includes(search));

      const statusBadge = (s) => {
        const cls = s === 'linked' ? 'badge-ok' : (s === 'missing' ? 'badge-warn' : 'badge-error');
        return `<span class="badge ${cls}">${s}</span>`;
      };

      table.innerHTML = `
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-gray-500 border-b">
              <th class="py-2 pr-4">Source</th>
              <th class="py-2 pr-4">Target</th>
              <th class="py-2 pr-4">Type</th>
              <th class="py-2">Status</th>
            </tr>
          </thead>
          <tbody>
            ${entries.map(e => `
              <tr class="border-b border-gray-100 hover:bg-gray-50">
                <td class="py-2 pr-4 font-mono text-xs">${esc(e.source)}</td>
                <td class="py-2 pr-4 font-mono text-xs">${esc(e.target)}</td>
                <td class="py-2 pr-4"><span class="badge ${e.isExternal ? 'badge-info' : 'badge-ok'}">${e.isExternal ? 'external' : 'native'}</span></td>
                <td class="py-2">${statusBadge(e.status)}</td>
              </tr>
            `).join('')}
          </tbody>
        </table>
        ${entries.length === 0 ? '<p class="text-gray-400 text-sm py-4 text-center">No entries match</p>' : ''}
      `;
    };

    filterSelect.onchange = renderTable;
    searchInput.oninput = renderTable;
    renderTable();
    root.appendChild(table);

  } catch (e) {
    root.innerHTML = `<div class="card"><p class="text-red-600">Error: ${esc(e.message)}</p></div>`;
  }
});

// esc() is defined in app.js
