// Классы приходят с башки машинными именами: показывать их человеку незачем
export const CLASS_LABELS = {
  high_fanout: 'много адресов сразу',
  geo_spread: 'работа из разных мест',
  port_scan: 'сканирование портов',
  mail_port: 'почтовые порты',
  torrent: 'торренты',
  malware: 'вредонос',
  upload_heavy: 'тяжёлая отдача',
  ads: 'реклама',
  no_device_id: 'клиент не назвался',
  no_receipts: 'трафик без расписок',
  address_mismatch: 'адрес не сходится',
  client_spread: 'разные клиенты на одной ссылке',
};

// Причины, по которым режут выплату НОДЕ. Шкала у них своя, поэтому и словарь
// отдельный от людского
export const NODE_REASONS = {
  overclaim: 'завышенный трафик',
  probe_fail: 'не пускает трафик из страны',
  flapping: 'то есть, то нет',
  profile_drop: 'не обслуживает профили',
  self_dealing: 'возит трафик сам себе',
};

export function bandLabel(band) {
  if (band === 'full') return 'полный';
  if (band === 'reduced') return 'урезанный';
  if (band === 'quarantine') return 'карантин';
  return band;
}

export function bandClass(band) {
  if (band === 'full') return 'is-online';
  if (band === 'reduced') return 'is-info';
  return 'is-offline';
}

export function bandIcon(band) {
  if (band === 'full') return '/img/oneui/security-high.svg';
  if (band === 'reduced') return '/img/oneui/security-medium.svg';
  return '/img/oneui/security-low.svg';
}
