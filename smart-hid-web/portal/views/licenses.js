export function licensesView() {
  const d = document.createElement('div');
  d.className = 'empty-state';
  d.innerHTML = `<h2>授权</h2><p class="muted">本页将在 CL-4c 实现。</p>`;
  return d;
}
