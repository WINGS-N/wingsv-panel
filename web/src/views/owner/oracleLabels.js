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
  flat_rhythm: 'активность без суточного ритма',
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

// Имена признаков по-человечески. В снимке они машинные, а владельцу надо
// понимать, что он смотрит, не лазая в исходники
export const FEATURE_LABELS = {
  requests: 'обращений',
  domains: 'разных имён',
  domains_per_hour: 'имён в час',
  bytes_per_request: 'байт на обращение',
  up_ratio: 'доля отдачи',
  long_lived_share: 'доля долгих',
  quiet_hours: 'самая длинная пауза, ч',
  active_hours: 'часов активности',
  spread_hours: 'размах, ч',
  random_name_share: 'доля машинных имён',
  no_domain_share: 'доля голых адресов',
  relay_port_hits: 'на почтовый релей',
  submission_hits: 'отправок почты',
  submission_targets: 'почтовых серверов',
  peer_port_hits: 'на порты bittorrent',
  distinct_bare_peers: 'разных пиров',
  ports_touched: 'разных портов',
  distinct_prints: 'разных TLS-стеков',
  top_print_share: 'доля главного стека',
  fresh_domains: 'свежих доменов',
  fresh_domain_share: 'доля свежих',
};
