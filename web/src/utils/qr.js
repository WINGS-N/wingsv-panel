// Код с иконкой WINGS V в середине. Один рисовальщик на всю панель: копии
// разъезжаются по размеру и уровню коррекции, и код в одном месте вдруг
// перестаёт читаться там, где в другом читался

const ICON_SRC = '/img/wingsv-icon.webp';

// Рисует код на переданный canvas. Уровень коррекции H держим намеренно: иконка
// закрывает середину, и без запаса код становится нечитаемым
export async function drawQR(canvas, link, size = 320) {
  if (!canvas || !link) return;
  const QR = await import('qrcode');
  canvas.width = size;
  canvas.height = size;
  await QR.toCanvas(canvas, link, {
    errorCorrectionLevel: 'H',
    width: size,
    margin: 1,
    color: { dark: '#000000', light: '#ffffff' },
  });

  const ctx = canvas.getContext('2d');
  const icon = new Image();
  icon.src = ICON_SRC;
  await new Promise((resolve) => {
    icon.onload = resolve;
    icon.onerror = resolve;
  });

  const badge = Math.round(size * 0.2);
  const x = (canvas.width - badge) / 2;
  const y = (canvas.height - badge) / 2;
  const pad = Math.round(badge * 0.1);
  ctx.fillStyle = '#ffffff';
  ctx.beginPath();
  ctx.roundRect(x - pad, y - pad, badge + pad * 2, badge + pad * 2, Math.round(badge * 0.25));
  ctx.fill();
  ctx.save();
  ctx.beginPath();
  ctx.roundRect(x, y, badge, badge, Math.round(badge * 0.22));
  ctx.clip();
  ctx.drawImage(icon, x, y, badge, badge);
  ctx.restore();
}

// Тот же код картинкой - для мест, где его вставляют в img
export async function generateQR(link) {
  if (!link) return '';
  try {
    const canvas = document.createElement('canvas');
    await drawQR(canvas, link);
    return canvas.toDataURL('image/png');
  } catch {
    return '';
  }
}
