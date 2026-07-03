// Shared formatting helpers for the stats/dashboard views.

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];

// formatBytes renders a byte count as a human-readable size (base-1024).
export function formatBytes(bytes) {
  let n = Number(bytes) || 0;
  if (n < 1) return '0 B';
  let i = 0;
  while (n >= 1024 && i < UNITS.length - 1) {
    n /= 1024;
    i += 1;
  }
  const digits = n >= 100 || i === 0 ? 0 : 1;
  return `${n.toFixed(digits)} ${UNITS[i]}`;
}

// formatUnix renders a unix-seconds timestamp in the ru-RU locale, or a dash.
export function formatUnix(seconds) {
  const s = Number(seconds) || 0;
  if (s <= 0) return '—';
  try {
    return new Date(s * 1000).toLocaleString('ru-RU');
  } catch {
    return String(s);
  }
}
