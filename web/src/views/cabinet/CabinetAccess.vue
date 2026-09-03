<template>
  <section class="surface-card">
    <div class="federation-live-head">
      <h2 class="section-title">Мой доступ</h2>
      <span v-if="trust" class="admin-pill" :class="bandClass(trust.band)">{{ bandLabel(trust.band) }}</span>
    </div>
    <p class="body-copy body-copy-wide">
      Бесплатный доступ выдаётся из общего пула серверов, которые отдали администраторы. Сколько серверов достанется,
      решает уровень доверия аккаунта.
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <p v-else-if="!enabled" class="state-hint">Федерация выключена, выдавать пока нечего.</p>

    <div v-if="enabled" class="admin-stats">
      <div class="stat">
        <span class="stat-kicker">
          <Server :size="14" class="stat-kicker-icon" aria-hidden="true" />
          Серверов
        </span>
        <span class="stat-value">{{ access.nodes }}</span>
        <span class="stat-meta">{{ nodesMeta }}</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <Gauge :size="14" class="stat-kicker-icon" aria-hidden="true" />
          Скорость
        </span>
        <span class="stat-value speed-pair">
          <span class="traffic-rx" aria-hidden="true">&darr;</span>{{ formatSpeed(access.downlink_bps) }}
          <span class="traffic-tx" aria-hidden="true">&uarr;</span>{{ formatSpeed(access.uplink_bps) }}
        </span>
        <span class="stat-meta">потолок по оценке доверия</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <ArrowDownUp :size="14" class="stat-kicker-icon" aria-hidden="true" />
          Передано
        </span>
        <span class="stat-value">{{ formatBytes(access.used_bytes) }}</span>
        <span class="stat-meta">за текущий месяц</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <img src="/img/oneui/security-high.svg" alt="" class="stat-kicker-img" aria-hidden="true" />
          Доверие
        </span>
        <span class="stat-value">{{ trust ? trust.confidence : '-' }}</span>
        <span class="stat-meta">{{ trust ? bandMeaning(trust.band) : 'оценка появится с первой сессией' }}</span>
      </div>
    </div>

    <div v-if="enabled && access.subscription_url" class="entry-card mt-4">
      <p class="body-copy">
        Войдите в WINGS Account в приложении - серверы появятся сами и будут обновляться. Добавлять и вставлять ничего
        не нужно.
      </p>
    </div>
  </section>

  <section v-if="trust && trust.classes.length" class="surface-card mt-6">
    <h2 class="section-title">Что снижает доверие</h2>
    <div class="fed-months mt-4">
      <div v-for="c in trust.classes" :key="c.kind" class="fed-month">
        <span class="fed-month-name">{{ classLabel(c.kind) }}</span>
        <span class="fed-month-track" aria-hidden="true">
          <span class="fed-month-fill" :style="{ width: classPct(c) + '%' }"></span>
        </span>
        <span class="fed-month-value">-{{ Math.round(c.weight) }}</span>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { ArrowDownUp, Gauge, Server } from 'lucide-vue-next';
import { formatSpeed } from '@/utils/format.js';

const CLASS_LABELS = {
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

const enabled = ref(false);
const loadError = ref('');
const access = reactive({
  nodes: 0,
  nodes_entitled: 0,
  used_bytes: 0,
  uplink_bps: 0,
  downlink_bps: 0,
  subscription_url: '',
  sticky_until: 0,
  import_link: '',
});
const trustRaw = ref(null);
const trust = computed(() => trustRaw.value);
// Когда выдано меньше положенного, дело не в доверии: две ноды одного донора
// падают вместе, поэтому вторую такую не дают
// Выданное и положенное расходятся в обе стороны: серверов может не хватать на
// всех, а может уже выданное держаться закреплением, пока доверие просело
const nodesMeta = computed(() => {
  const entitled = Number(access.nodes_entitled || 0);
  if (entitled > access.nodes) {
    return `из ${entitled} по доверию - остальные пока некому отдать`;
  }
  if (entitled > 0 && entitled < access.nodes) {
    return `закреплены за вами, по доверию сейчас положено ${entitled}`;
  }
  return 'выдано вам сейчас';
});

onMounted(load);

async function load() {
  try {
    const res = await fetch('/api/admin/me/access', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    enabled.value = Boolean(data.enabled);
    access.nodes = data.nodes || 0;
    access.subscription_url = data.subscription_url || '';
    access.sticky_until = data.sticky_until || 0;
    access.import_link = data.import_link || '';
    access.used_bytes = data.used_bytes || 0;
    access.nodes_entitled = data.nodes_entitled || 0;
    access.uplink_bps = data.uplink_bps || 0;
    access.downlink_bps = data.downlink_bps || 0;
    trustRaw.value = data.trust ? { ...data.trust, classes: data.trust.classes || [] } : null;
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  }
}

function classLabel(kind) {
  return CLASS_LABELS[kind] || kind;
}

function classPct(entry) {
  const top = Math.max(...(trust.value?.classes || []).map((c) => Number(c.weight) || 0), 1);
  return Math.max(2, Math.round((Number(entry.weight) / top) * 100));
}

function bandLabel(band) {
  if (band === 'full') return 'полный доступ';
  if (band === 'reduced') return 'урезанный доступ';
  if (band === 'quarantine') return 'карантин';
  return band;
}

// Потолок скорости соразмерен оценке доверия, а не полосе

// Единицы английские, как везде в проекте
function formatBytes(value) {
  const bytes = Number(value || 0);
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let size = bytes / 1024;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(1)} ${units[unit]}`;
}

function bandMeaning(band) {
  if (band === 'full') return 'полная скорость и несколько серверов';
  if (band === 'reduced') return 'скорость урезана, серверов меньше';
  return 'доступ приостановлен';
}

function bandClass(band) {
  if (band === 'full') return 'is-online';
  if (band === 'reduced') return 'is-info';
  return 'is-offline';
}
</script>

<style scoped>
.speed-pair {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  white-space: nowrap;
}

.speed-pair .traffic-rx,
.speed-pair .traffic-tx {
  font-size: 0.85em;
}
</style>
