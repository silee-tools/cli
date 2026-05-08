App.registerPage('dashboard', async (root) => {
  try {
    const data = await Api.get('/api/status');
    root.innerHTML = '';

    // Quick actions
    const actions = h('div', { className: 'flex gap-3 mb-6' },
      h('button', { className: 'btn btn-primary', onclick: async () => {
        try {
          const r = await Api.post('/api/links/link', { dryRun: App.isDryRun() });
          toast(`Linked: ${r.report.linked}, Skipped: ${r.report.skipped}, Failed: ${r.report.failed}`,
            r.report.failed > 0 ? 'error' : 'success');
          App.route();
        } catch (e) { toast(e.message, 'error'); }
      }}, 'Link All'),
      h('button', { className: 'btn btn-secondary', onclick: async () => {
        try {
          const r = await Api.post('/api/sync', { dryRun: App.isDryRun() });
          const msgs = r.results.map(x => `${x.task}: ${x.success ? 'ok' : x.message}`).join(', ');
          toast(`Sync: ${msgs}`, r.results.every(x => x.success) ? 'success' : 'error');
        } catch (e) { toast(e.message, 'error'); }
      }}, 'Sync')
    );
    root.appendChild(actions);

    // Cards grid
    const grid = h('div', { className: 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4' });

    // Config card
    grid.appendChild(h('div', { className: 'card' },
      h('h3', { className: 'font-semibold text-sm text-gray-500 mb-3' }, 'Configuration'),
      h('p', { className: 'text-sm mb-1' },
        h('span', { className: 'text-gray-500' }, 'Config Dir: '),
        h('span', { className: 'font-mono text-xs' }, data.configDir)
      ),
      ...Object.entries(data.vars).map(([k, v]) =>
        h('p', { className: 'text-sm' },
          h('span', { className: 'text-gray-500' }, `$${k}: `),
          h('span', { className: 'font-mono text-xs' }, v)
        )
      )
    ));

    // Links card
    const linksPct = data.links.total > 0
      ? Math.round(((data.links.total - data.drift.total) / data.links.total) * 100) : 100;
    grid.appendChild(h('div', { className: 'card' },
      h('h3', { className: 'font-semibold text-sm text-gray-500 mb-3' }, 'Links'),
      h('div', { className: 'flex items-baseline gap-2 mb-2' },
        h('span', { className: 'text-3xl font-bold' }, String(data.links.total)),
        h('span', { className: 'text-sm text-gray-500' }, 'total')
      ),
      h('div', { className: 'flex gap-4 text-sm mb-3' },
        h('span', { innerHTML: `<span class="badge badge-info">Native ${data.links.native}</span>` }),
        h('span', { innerHTML: `<span class="badge badge-ok">Custom ${data.links.custom}</span>` })
      ),
      h('div', { className: 'w-full bg-gray-200 rounded-full h-2' },
        h('div', { className: `h-2 rounded-full ${linksPct === 100 ? 'bg-green-500' : 'bg-yellow-500'}`,
          style: `width: ${linksPct}%` })
      ),
      h('p', { className: 'text-xs text-gray-500 mt-1' }, `${linksPct}% in sync`)
    ));

    // Drift card
    const driftColor = data.drift.total === 0 ? 'text-green-600' : 'text-red-600';
    grid.appendChild(h('div', { className: 'card' },
      h('h3', { className: 'font-semibold text-sm text-gray-500 mb-3' }, 'Drift'),
      h('div', { className: `text-3xl font-bold ${driftColor} mb-2` }, String(data.drift.total)),
      ...(data.drift.total > 0 ? [
        h('div', { className: 'space-y-1 text-sm' },
          ...[
            ['Broken', data.drift.broken, 'badge-error'],
            ['Missing', data.drift.missing, 'badge-warn'],
            ['Wrong Target', data.drift.wrongTarget, 'badge-error'],
            ['Not Symlink', data.drift.notSymlink, 'badge-warn']
          ].filter(x => x[1] > 0).map(([label, count, cls]) =>
            h('div', {}, h('span', { className: `badge ${cls}` }, `${label}: ${count}`))
          )
        )
      ] : [h('p', { className: 'text-sm text-green-600' }, 'All links in sync')])
    ));

    // Native Dirs card (spans full width)
    const ndTable = h('table', { className: 'w-full text-sm' },
      h('thead', {},
        h('tr', { className: 'text-left text-gray-500 border-b' },
          h('th', { className: 'py-1 pr-4' }, 'Directory'),
          h('th', { className: 'py-1 pr-4' }, 'Target'),
          h('th', { className: 'py-1 pr-4' }, 'Mode'),
          h('th', { className: 'py-1 pr-4' }, 'Status'),
          h('th', { className: 'py-1' }, 'Files')
        )
      ),
      h('tbody', {},
        ...data.nativeDirs.map(nd =>
          h('tr', { className: 'border-b border-gray-100' },
            h('td', { className: 'py-1 pr-4 font-mono' }, nd.dir + '/'),
            h('td', { className: 'py-1 pr-4 font-mono text-xs' }, nd.targetBase || '-'),
            h('td', { className: 'py-1 pr-4' }, h('span', { className: 'badge badge-info' }, nd.mode)),
            h('td', { className: 'py-1 pr-4' },
              h('span', { className: `badge ${nd.exists ? 'badge-ok' : 'badge-warn'}` },
                nd.exists ? 'exists' : 'missing')
            ),
            h('td', { className: 'py-1' }, String(nd.fileCount))
          )
        )
      )
    );

    const ndCard = h('div', { className: 'card col-span-full' },
      h('h3', { className: 'font-semibold text-sm text-gray-500 mb-3' }, 'Native Directories'),
      ndTable
    );
    grid.appendChild(ndCard);

    root.appendChild(grid);
  } catch (e) {
    root.innerHTML = `<div class="card"><p class="text-red-600">Error: ${esc(e.message)}</p></div>`;
  }
});
