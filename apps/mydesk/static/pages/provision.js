App.registerPage('provision', async (root) => {
  try {
    const data = await Api.get('/api/provision/status');
    root.innerHTML = '';

    const grid = h('div', { className: 'grid grid-cols-1 md:grid-cols-2 gap-4' });

    // Homebrew
    grid.appendChild(provisionCard('Homebrew', 'Brewfile', data.brew, [
      { label: 'Sync', action: 'brew-sync', desc: 'Export current packages to Brewfile' },
      { label: 'Install', action: 'brew-install', desc: 'Install packages from Brewfile' }
    ]));

    // VS Code
    const vscodeExtra = data.vscode.extensionCount > 0
      ? ` (${data.vscode.extensionCount} extensions)` : '';
    grid.appendChild(provisionCard('VS Code', 'extensions.txt' + vscodeExtra, data.vscode, [
      { label: 'Sync', action: 'vscode-sync', desc: 'Export installed extensions' },
      { label: 'Install', action: 'vscode-install', desc: 'Install extensions from list' }
    ]));

    // Oh-My-Zsh
    grid.appendChild(provisionCard('Oh-My-Zsh', 'install.sh', data.omz, [
      { label: 'Install', action: 'omz-install', desc: 'Run Oh-My-Zsh setup script' }
    ]));

    // macOS Defaults
    grid.appendChild(provisionCard('macOS Defaults', 'defaults.sh', data.macos, [
      { label: 'Apply', action: 'apply-defaults', desc: 'Apply system preferences' }
    ]));

    // mise
    grid.appendChild(provisionCard('mise', 'runtime manager', data.mise, [
      { label: 'Install', action: 'mise-install', desc: 'Install configured runtimes' }
    ]));

    root.appendChild(grid);
  } catch (e) {
    root.innerHTML = `<div class="card"><p class="text-red-600">Error: ${esc(e.message)}</p></div>`;
  }
});

function provisionCard(title, subtitle, status, actions) {
  const card = h('div', { className: 'card' });

  const header = h('div', { className: 'flex items-center justify-between mb-3' },
    h('div', {},
      h('h3', { className: 'font-semibold' }, title),
      h('p', { className: 'text-xs text-gray-500' }, subtitle)
    ),
    h('span', { className: `badge ${status.available ? 'badge-ok' : 'badge-warn'}` },
      status.available ? 'Available' : 'Not found')
  );
  card.appendChild(header);

  if (status.path) {
    card.appendChild(h('p', { className: 'text-xs font-mono text-gray-400 mb-3 truncate', title: status.path }, status.path));
  }

  const btnRow = h('div', { className: 'flex gap-2' });
  for (const act of actions) {
    const btn = h('button', {
      className: 'btn btn-secondary',
      title: act.desc,
      onclick: async () => {
        try {
          btn.disabled = true;
          btn.textContent = act.label + '...';
          const r = await Api.post(`/api/provision/${act.action}`, { dryRun: App.isDryRun() });
          toast(`${title}: ${r.message}`, 'success');
        } catch (e) {
          toast(`${title}: ${e.message}`, 'error');
        } finally {
          btn.disabled = false;
          btn.textContent = act.label;
        }
      }
    }, act.label);
    btnRow.appendChild(btn);
  }
  card.appendChild(btnRow);

  return card;
}
