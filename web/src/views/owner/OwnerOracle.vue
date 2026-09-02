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
      <div v-for="s in pagedSubjects" :key="s.subject_id" class="fed-node-row is-tappable" @click="open(s)">
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
      </div>
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

  <SamsungModal :model-value="detailOpen" title="Профиль под наблюдением" @update:model-value="closeDetail">
    <p v-if="detailName" class="mt-1 text-[16px]">{{ detailName }}</p>
    <p class="admin-mono mt-1 break-all text-[13px] text-wings-muted">{{ detailId }}</p>
    <p v-if="detailError" class="state-error mt-3">{{ detailError }}</p>
    <SamsungSectionLoader v-else-if="!detail" />
    <template v-else>
      <p class="admin-muted mt-2">
        Доверие {{ detail.subject.confidence }}, полоса {{ bandLabel(detail.subject.band) }}. Каждое наблюдение с
        временем и нодой, на которой оно случилось.
      </p>
      <div v-if="detail.signals.length" class="fed-node-list mt-4">
        <div v-for="(sig, i) in detail.signals" :key="i" class="fed-node-row">
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
      <p v-else class="state-hint mt-4">Сырых сигналов не осталось, их вес истёк.</p>

      <h3 class="section-title mt-6 text-[15px]">Куда ходил</h3>
      <p class="admin-muted mt-1">Чаще всего за неделю. Наблюдения хранятся месяц.</p>
      <div v-if="detail.domains && detail.domains.length" class="fed-node-list mt-3">
        <div v-for="d in detail.domains" :key="d.domain" class="fed-node-row">
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
      <p v-else class="state-hint mt-3">Наблюдений нет.</p>
    </template>
    <template #actions>
      <SamsungButton variant="secondary" @click="closeDetail">Закрыть</SamsungButton>
    </template>
  </SamsungModal>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { ChevronRight } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungModal from '@/components/layout/SamsungModal.vue';
import SamsungPager from '@/components/controls/SamsungPager.vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';

const enabled = ref(false);
const scorer = ref('');
const loadError = ref('');
const detail = ref(null);
const detailOpen = ref(false);
const detailId = ref('');
const detailName = ref('');
const detailError = ref('');
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

const NODE_REASONS = {
  overclaim: 'завышенный трафик',
  probe_fail: 'не пускает трафик из страны',
  flapping: 'то есть, то нет',
  profile_drop: 'не обслуживает профили',
};

const page = ref(1);
const perPage = 20;
const pagedSubjects = computed(() => overview.subjects.slice((page.value - 1) * perPage, page.value * perPage));
let timer = null;

// Классы приходят с головы машинными именами: показывать их человеку незачем
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
};

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

async function open(subject) {
  detailOpen.value = true;
  detailId.value = subject.subject_id;
  detailName.value = subject.username || '';
  detail.value = null;
  detailError.value = '';
  try {
    const res = await fetch(`/api/owner/federation/oracle/subject?id=${encodeURIComponent(subject.subject_id)}`, {
      credentials: 'include',
    });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    // Пока ждали ответ, модалку могли закрыть или открыть другого
    if (detailOpen.value && detailId.value === subject.subject_id) detail.value = data;
  } catch (err) {
    if (detailOpen.value && detailId.value === subject.subject_id) detailError.value = String(err.message || err);
  }
}

function closeDetail() {
  detailOpen.value = false;
  detail.value = null;
  detailError.value = '';
}

function classLabel(kind) {
  return CLASS_LABELS[kind] || kind;
}

function signalPct(signal) {
  const top = Math.max(...overview.signals.map((s) => Number(s.count) || 0), 1);
  return Math.max(2, Math.round((Number(signal.count) / top) * 100));
}

function bandLabel(band) {
  if (band === 'full') return 'полный';
  if (band === 'reduced') return 'урезанный';
  if (band === 'quarantine') return 'карантин';
  return band;
}

function bandClass(band) {
  if (band === 'full') return 'is-online';
  if (band === 'reduced') return 'is-info';
  return 'is-offline';
}

function bandIcon(band) {
  if (band === 'full') return '/img/oneui/security-high.svg';
  if (band === 'reduced') return '/img/oneui/security-medium.svg';
  return '/img/oneui/security-low.svg';
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
