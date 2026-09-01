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
        <span class="stat-value">без лимита</span>
        <span class="stat-meta">упирается только в сам сервер</span>
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
        Откройте на телефоне с установленным WINGS V - подписка заведётся сама и будет обновляться.
      </p>
      <div class="actions-row">
        <SamsungButton v-if="access.import_link" @click="openInApp">
          <template #icon><Smartphone class="button-icon" aria-hidden="true" /></template>
          Добавить в приложение
        </SamsungButton>
      </div>
      <p class="admin-muted mt-3">Или вставьте ссылку вручную:</p>
      <CopyableLink :value="access.subscription_url" class="mt-2" />
    </div>
  </section>

  <section v-if="trust && trust.classes.length" class="surface-card mt-6">
    <h2 class="section-title">Что снижает доверие</h2>
    <p class="admin-muted mt-1">
      Считается только форма поведения: сколько адресов, откуда и какие порты. Что вы открываете, не смотрит никто.
    </p>
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
import { ArrowDownUp, Gauge, Server, Smartphone } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import CopyableLink from '@/components/domain/CopyableLink.vue';

const CLASS_LABELS = {
  high_fanout: 'много адресов сразу',
  geo_spread: 'работа из разных мест',
  port_scan: 'сканирование портов',
  mail_port: 'почтовые порты',
  torrent: 'торренты',
  malware: 'вредонос',
  upload_heavy: 'тяжёлая отдача',
  ads: 'реклама',
};

const enabled = ref(false);
const loadError = ref('');
const access = reactive({
  nodes: 0,
  nodes_entitled: 0,
  used_bytes: 0,
  subscription_url: '',
  sticky_until: 0,
  import_link: '',
});
const trustRaw = ref(null);
const trust = computed(() => trustRaw.value);
// Когда выдано меньше положенного, дело не в доверии: две ноды одного донора
// падают вместе, поэтому вторую такую не дают
const nodesMeta = computed(() => {
  const entitled = Number(access.nodes_entitled || 0);
  if (entitled > access.nodes) {
    return `из ${entitled} по доверию - остальные пока некому отдать`;
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
    trustRaw.value = data.trust ? { ...data.trust, classes: data.trust.classes || [] } : null;
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  }
}

// Ссылка открывается своей схемой: приложение перехватывает её и заводит
// подписку, а браузер остаётся на месте
function openInApp() {
  window.location.href = access.import_link;
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
  if (band === 'full') return 'несколько серверов на выбор';
  if (band === 'reduced') return 'один сервер и меньше скорость';
  return 'доступ приостановлен';
}

function bandClass(band) {
  if (band === 'full') return 'is-online';
  if (band === 'reduced') return 'is-info';
  return 'is-offline';
}
</script>
