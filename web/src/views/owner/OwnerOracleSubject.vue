<template>
  <section class="surface-card">
    <header class="admin-card-header admin-detail-header">
      <div>
        <router-link class="admin-back-link" :to="{ name: 'owner-oracle' }">← Oracle</router-link>
        <h1 class="admin-card-title">{{ subject?.username || id }}</h1>
        <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-[13px] text-wings-muted">
          <code class="admin-mono">{{ id }}</code>
          <template v-if="subject">
            <span>·</span>
            <span class="admin-pill" :class="bandClass(subject.band)">{{ bandLabel(subject.band) }}</span>
            <span>доверие {{ subject.confidence }}</span>
            <span v-if="subject.shadow_band && subject.shadow_band !== subject.band" class="admin-pill">
              теневой: {{ bandLabel(subject.shadow_band) }}
            </span>
          </template>
        </div>
      </div>
    </header>
    <p v-if="loadError" class="state-error mt-3">{{ loadError }}</p>
    <SamsungSectionLoader v-else-if="!subject" />
    <div v-else-if="subject.classes.length" class="fed-months mt-4">
      <div v-for="c in subject.classes" :key="c.kind" class="fed-month">
        <span class="fed-month-name">{{ classLabel(c.kind) }}</span>
        <span class="fed-month-track" aria-hidden="true">
          <span class="fed-month-fill" :style="{ width: classPct(c) + '%' }"></span>
        </span>
        <span class="fed-month-value">-{{ Math.round(c.weight) }}</span>
      </div>
    </div>
  </section>

  <section class="surface-card mt-6">
    <div class="federation-live-head">
      <h2 class="section-title">Куда ходил</h2>
      <div class="admin-tabs">
        <button
          v-for="w in windows"
          :key="w.hours"
          :class="['admin-tab', windowHours === w.hours ? 'is-active' : '']"
          @click="setWindow(w.hours)"
        >
          {{ w.label }}
        </button>
      </div>
    </div>
    <p class="admin-muted mt-1">Наблюдения хранятся месяц, дальше выносятся сами.</p>
    <SamsungSectionLoader v-if="domainsLoading" />
    <p v-else-if="!domains.length" class="state-hint mt-4">Наблюдений нет.</p>
    <div v-else class="fed-node-list mt-4">
      <div v-for="d in domains" :key="d.domain" class="fed-node-row">
        <div class="min-w-0 flex-1">
          <span class="admin-mono block truncate text-[14px]">{{ d.domain }}</span>
          <span class="mt-1 flex flex-wrap items-center gap-3 text-[13px] text-wings-muted">
            <span>{{ d.hits }} раз</span>
            <span>{{ formatBytes(Number(d.down_bytes || 0)) }} вниз</span>
            <span>{{ formatBytes(Number(d.up_bytes || 0)) }} вверх</span>
            <span>{{ when(d.last_seen_unix) }}</span>
          </span>
        </div>
      </div>
    </div>
    <SamsungPager v-model:page="domainPage" :total="domainsTotal" :per-page="DOMAIN_PER_PAGE" />
  </section>

  <section class="surface-card mt-6">
    <h2 class="section-title">Сигналы</h2>
    <p class="admin-muted mt-1">Каждое наблюдение с временем и нодой, на которой оно случилось.</p>
    <SamsungSectionLoader v-if="signalsLoading" />
    <p v-else-if="!signals.length" class="state-hint mt-4">Сырых сигналов не осталось, их вес истёк.</p>
    <div v-else class="fed-node-list mt-4">
      <div v-for="(sig, i) in signals" :key="i" class="fed-node-row">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-[15px]">{{ classLabel(sig.kind) }}</span>
            <span class="admin-pill">{{ sig.count }}</span>
          </div>
          <span class="mt-1 flex flex-wrap items-center gap-3 text-[13px] text-wings-muted">
            <span>{{ when(sig.at_unix) }}</span>
            <span v-if="sig.node_id">нода {{ sig.node_id.slice(0, 8) }}</span>
          </span>
        </div>
      </div>
    </div>
    <SamsungPager v-model:page="signalPage" :total="signalsTotal" :per-page="SIGNAL_PER_PAGE" />
  </section>
</template>

<script setup>
import { computed, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import SamsungPager from '@/components/controls/SamsungPager.vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';
import { CLASS_LABELS, bandClass, bandLabel } from './oracleLabels';

const DOMAIN_PER_PAGE = 30;
const SIGNAL_PER_PAGE = 25;

const route = useRoute();
const id = computed(() => String(route.params.id || ''));

const subject = ref(null);
const domains = ref([]);
const signals = ref([]);
const domainsTotal = ref(0);
const signalsTotal = ref(0);
const domainPage = ref(1);
const signalPage = ref(1);
const domainsLoading = ref(true);
const signalsLoading = ref(true);
const loadError = ref('');
const windowHours = ref(24 * 7);
const windows = [
  { hours: 24, label: 'Сутки' },
  { hours: 24 * 7, label: 'Неделя' },
  { hours: 24 * 30, label: 'Месяц' },
];

// Каждый список тянет свою страницу отдельно, иначе листание доменов гоняло бы
// ещё и сигналы, а на длинном хвосте это заметно
watch([id, domainPage, windowHours], () => load('domains'), { immediate: true });
watch([id, signalPage], () => load('signals'), { immediate: true });

async function load(part) {
  if (!id.value) return;
  const loading = part === 'domains' ? domainsLoading : signalsLoading;
  loading.value = true;
  const params = new URLSearchParams({ id: id.value, window_hours: String(windowHours.value) });
  if (part === 'domains') {
    params.set('limit', String(DOMAIN_PER_PAGE));
    params.set('offset', String((domainPage.value - 1) * DOMAIN_PER_PAGE));
    params.set('signal_limit', '1');
  } else {
    params.set('limit', '1');
    params.set('signal_limit', String(SIGNAL_PER_PAGE));
    params.set('signal_offset', String((signalPage.value - 1) * SIGNAL_PER_PAGE));
  }
  const asked = id.value;
  try {
    const res = await fetch(`/api/owner/federation/oracle/subject?${params}`, { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    // Пока ждали ответ, могли уйти на другого наблюдаемого
    if (asked !== id.value) return;
    subject.value = data.subject || null;
    if (part === 'domains') {
      domains.value = data.domains || [];
      domainsTotal.value = Number(data.domains_total || 0);
    } else {
      signals.value = data.signals || [];
      signalsTotal.value = Number(data.signals_total || 0);
    }
    loadError.value = '';
  } catch (err) {
    if (asked === id.value) loadError.value = String(err.message || err);
  } finally {
    loading.value = false;
  }
}

function setWindow(hours) {
  windowHours.value = hours;
  domainPage.value = 1;
}

function classLabel(kind) {
  return CLASS_LABELS[kind] || kind;
}

function classPct(item) {
  const top = Math.max(...(subject.value?.classes || []).map((c) => Number(c.weight) || 0), 1);
  return Math.max(2, Math.round((Number(item.weight) / top) * 100));
}

function formatBytes(bytes) {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

function when(unix) {
  if (!unix) return '';
  return new Date(Number(unix) * 1000).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' });
}
</script>
