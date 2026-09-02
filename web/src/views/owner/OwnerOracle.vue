<template>
  <section class="surface-card">
    <div class="federation-live-head">
      <h2 class="section-title">Oracle</h2>
      <span v-if="scorer" class="admin-pill is-info">{{ scorer }}</span>
    </div>
    <p class="body-copy body-copy-wide">
      Доверие есть у каждого участника и двигается само: подозрительное поведение опускает его, спокойное поднимает, вес
      сигналов со временем тает. Считается и поведение, и то, куда шли соединения.
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <p v-else-if="!enabled" class="state-hint">Федерация выключена.</p>

    <div v-if="enabled" class="admin-tabs mt-4">
      <button
        v-for="item in tabs"
        :key="item.id"
        :class="['admin-tab', tab === item.id ? 'is-active' : '']"
        @click="setTab(item.id)"
      >
        {{ item.label }}
      </button>
    </div>

    <div v-if="enabled && tab === 'people'" class="admin-stats">
      <div class="stat">
        <span class="stat-kicker">
          <img src="/img/oneui/preferences-system-privacy.svg" alt="" class="stat-kicker-img" aria-hidden="true" />
          Наблюдаемые обвиняемые
        </span>
        <span class="stat-value">{{ overview.watched }}</span>
        <span class="stat-meta">из {{ overview.subjects_total }} с доступом</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <img src="/img/oneui/security-high.svg" alt="" class="stat-kicker-img" aria-hidden="true" />
          Полный доступ
        </span>
        <span class="stat-value">{{ overview.full }}</span>
        <span class="stat-meta">несколько серверов на выбор</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <img src="/img/oneui/security-medium.svg" alt="" class="stat-kicker-img" aria-hidden="true" />
          Урезанный
        </span>
        <span class="stat-value">{{ overview.reduced }}</span>
        <span class="stat-meta">один сервер и меньше скорость</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <img src="/img/oneui/security-low.svg" alt="" class="stat-kicker-img" aria-hidden="true" />
          Карантин
        </span>
        <span class="stat-value">{{ overview.quarantined }}</span>
        <span class="stat-meta">доступ снят</span>
      </div>
    </div>
  </section>

  <section v-if="enabled && tab === 'people' && overview.signals.length" class="surface-card mt-6">
    <h2 class="section-title">Сигналы за сутки</h2>
    <p class="admin-muted mt-1">Доля класса среди всех сигналов и скольких участников он задел.</p>
    <div class="fed-months mt-4">
      <div v-for="s in overview.signals" :key="s.kind" class="fed-month">
        <span class="fed-month-name">{{ classLabel(s.kind) }}</span>
        <span class="fed-month-track" aria-hidden="true">
          <span class="fed-month-fill" :style="{ width: (s.share_pct || signalPct(s)) + '%' }"></span>
        </span>
        <span class="fed-month-value">
          {{ Math.round(s.share_pct || 0) }}%
          <span class="admin-muted">· {{ s.count }} у {{ s.subjects || 0 }}</span>
        </span>
      </div>
    </div>
  </section>

  <section v-if="enabled && tab === 'people'" class="surface-card mt-6">
    <h2 class="section-title">Наблюдаемые</h2>
    <p v-if="!overview.subjects.length" class="state-hint">Пока ни на кого ничего нет.</p>
    <div v-else class="fed-node-list mt-4">
      <router-link
        v-for="s in pagedSubjects"
        :key="s.subject_id"
        class="fed-node-row is-tappable"
        :to="{ name: 'owner-oracle-subject', params: { id: s.subject_id } }"
      >
        <img :src="bandIcon(s.band)" alt="" class="probe-icon" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="truncate text-[15px]">{{ s.username || s.subject_id }}</span>
            <span v-if="s.username" class="admin-mono text-[12px] text-wings-muted">{{ s.subject_id }}</span>
            <span class="admin-pill" :class="bandClass(s.band)">{{ bandLabel(s.band) }}</span>
            <span
              v-if="s.shadow_band && s.shadow_band !== s.band"
              class="admin-pill"
              title="Теневой скорер ничего не решает"
            >
              теневой: {{ bandLabel(s.shadow_band) }}
            </span>
          </div>
          <span class="mt-1 flex flex-wrap items-center gap-3 text-[13px] text-wings-muted">
            <span>доверие {{ s.confidence }}</span>
            <span v-for="c in s.classes.slice(0, 3)" :key="c.kind">
              {{ classLabel(c.kind) }} -{{ Math.round(c.weight) }}
            </span>
          </span>
        </div>
        <ChevronRight :size="18" class="text-wings-muted" aria-hidden="true" />
      </router-link>
    </div>
  </section>

  <SamsungPager
    v-if="enabled && tab === 'people' && overview.subjects.length"
    v-model:page="page"
    :total="overview.subjects.length"
    :per-page="perPage"
  />

  <section v-if="enabled && tab === 'nodes'" class="surface-card mt-6">
    <div class="federation-live-head">
      <h2 class="section-title">Доверие к нодам</h2>
      <span v-if="nodes.accused" class="admin-pill is-offline">обвиняемых: {{ nodes.accused }}</span>
    </div>
    <p class="admin-muted mt-1">Цифры ноды против того, что подписали клиенты. От расхождения зависит начисление.</p>
    <SamsungSectionLoader v-if="nodesLoading" />
    <p v-else-if="!nodes.list.length" class="state-hint mt-4">Нод пока нет.</p>
    <div v-else class="fed-node-list mt-4">
      <div v-for="n in nodes.list" :key="n.node_id" class="fed-node-row">
        <img :src="trustIcon(n)" alt="" class="probe-icon" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="truncate text-[15px]">{{ n.hostname || n.node_id.slice(0, 12) }}</span>
            <span class="admin-pill" :class="trustClass(n)">доверие {{ n.trust }}</span>
            <span v-if="n.payout_stopped" class="admin-pill is-offline">выплата остановлена</span>
            <span v-else-if="n.payout_reduced" class="admin-pill is-info">выплата урезана</span>
          </div>
          <span class="mt-1 flex flex-wrap items-center gap-3 text-[13px] text-wings-muted">
            <span>замеры {{ n.probe_ok }} онлайн, {{ n.probe_failed }} мимо</span>
            <span>аптайм {{ Math.round(n.uptime_pct || 0) }}%</span>
            <span v-for="reason in n.reasons.slice(0, 3)" :key="reason.reason">
              {{ reasonLabel(reason.reason) }} -{{ Math.round(reason.weight) }}
            </span>
          </span>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { ChevronRight } from 'lucide-vue-next';
import SamsungPager from '@/components/controls/SamsungPager.vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';
import { CLASS_LABELS, NODE_REASONS, bandClass, bandIcon, bandLabel } from './oracleLabels';

const enabled = ref(false);
const scorer = ref('');
const loadError = ref('');
const overview = reactive({
  watched: 0,
  subjects_total: 0,
  full: 0,
  reduced: 0,
  quarantined: 0,
  subjects: [],
  signals: [],
});
// Ноды судим отдельной вкладкой: шкала у них своя и грехи свои
const tabs = [
  { id: 'people', label: 'Участники' },
  { id: 'nodes', label: 'Ноды' },
];
const tab = ref('people');
const nodesLoading = ref(false);
const nodes = reactive({ list: [], accused: 0 });

const page = ref(1);
const perPage = 20;
const pagedSubjects = computed(() => overview.subjects.slice((page.value - 1) * perPage, page.value * perPage));
let timer = null;

onMounted(() => {
  load();
  timer = setInterval(load, 20000);
});

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});

function setTab(id) {
  tab.value = id;
  if (id === 'nodes' && !nodes.list.length) {
    loadNodes();
  }
}

async function loadNodes() {
  nodesLoading.value = true;
  try {
    const res = await fetch('/api/owner/federation/oracle/nodes', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    nodes.list = data.nodes || [];
    nodes.accused = data.accused || 0;
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    nodesLoading.value = false;
  }
}

function reasonLabel(reason) {
  return NODE_REASONS[reason] || reason;
}

function trustClass(node) {
  if (node.payout_stopped) return 'is-offline';
  if (node.payout_reduced) return 'is-info';
  return 'is-online';
}

function trustIcon(node) {
  if (node.payout_stopped) return '/img/oneui/security-low.svg';
  if (node.payout_reduced) return '/img/oneui/security-medium.svg';
  return '/img/oneui/security-high.svg';
}

async function load() {
  try {
    const res = await fetch('/api/owner/federation/oracle', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    enabled.value = Boolean(data.enabled);
    scorer.value = data.scorer || '';
    Object.assign(overview, {
      watched: data.watched || 0,
      subjects_total: data.subjects_total || 0,
      full: data.full || 0,
      reduced: data.reduced || 0,
      quarantined: data.quarantined || 0,
      subjects: data.subjects || [],
      signals: data.signals || [],
    });
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  }
}

function classLabel(kind) {
  return CLASS_LABELS[kind] || kind;
}

function signalPct(signal) {
  const top = Math.max(...overview.signals.map((s) => Number(s.count) || 0), 1);
  return Math.max(2, Math.round((Number(signal.count) / top) * 100));
}
</script>
