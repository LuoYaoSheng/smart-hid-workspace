// 占位 —— CL-4b 实现。
export function dashboardView() {
  const d = document.createElement('div');
  d.className = 'empty-state';
  d.innerHTML = `<h2>概览</h2><p class="muted">本页将在 CL-4b 实现。</p>`;
  return d;
}
