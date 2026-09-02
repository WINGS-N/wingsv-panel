<template>
  <section class="surface-card">
    <div class="federation-live-head">
      <h2 class="section-title">Oracle</h2>
      <span v-if="scorer" class="admin-pill is-info">{{ scorer }}</span>
    </div>
    <p class="body-copy body-copy-wide">
      Уровень доверия есть у каждого профиля. Он начинается посередине и двигается сам: подозрительное поведение
      опускает его, спокойное поднимает обратно, потому что вес каждого сигнала со временем тает. Считается количество и
      форма, а не содержимое: ни одного домена сюда не приезжает.
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <p v-else-if="!enabled" class="state-hint">Федерация выключена.</p>

    <div v-if="enabled" class="admin-stats">
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

  <section v-if="enabled && overview.signals.length" class="surface-card mt-6">
    <h2 class="section-title">Сигналы за сутки</h2>
    <p class="admin-muted mt-1">
      На что уходит внимание оракула: доля класса среди всех сигналов и скольких профилей он касается. Что именно
      открывали, не знает никто.
    </p>
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

  <section v-if="enabled" class="surface-card mt-6">
    <h2 class="section-title">Наблюдаемые</h2>
    <p v-if="!overview.subjects.length" class="state-hint">Пока ни на кого ничего нет.</p>
    <div v-else class="fed-node-list mt-4">
      <div v-for="s in pagedSubjects" :key="s.subject_id" class="fed-node-row is-tappable" @click="open(s)">
        <img :src="bandIcon(s.band)" alt="" class="probe-icon" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="admin-mono truncate text-[15px]">{{ s.subject_id }}</span>
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
    v-if="enabled && overview.subjects.length"
    v-model:page="page"
    :total="overview.subjects.length"
    :per-page="perPage"
  />

  <section v-if="detail" class="surface-card mt-6">
    <div class="federation-live-head">
      <h2 class="section-title">{{ detail.subject.subject_id }}</h2>
      <SamsungButton variant="ghost" @click="detail = null">Закрыть</SamsungButton>
    </div>
    <p class="admin-muted mt-1">
      Доверие {{ detail.subject.confidence }}, полоса {{ bandLabel(detail.subject.band) }}. Каждое наблюдение с временем
      и нодой, на которой оно случилось.
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
    <p v-else class="state-hint">Сырых сигналов не осталось: их вес истёк.</p>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { ChevronRight } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungPager from '@/components/controls/SamsungPager.vue';

const enabled = ref(false);
const scorer = ref('');
const loadError = ref('');
const detail = ref(null);
const overview = reactive({
  watched: 0,
  subjects_total: 0,
  full: 0,
  reduced: 0,
  quarantined: 0,
  subjects: [],
  signals: [],
});
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
};

onMounted(() => {
  load();
  timer = setInterval(load, 20000);
});

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});

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
  try {
    const res = await fetch(`/api/owner/federation/oracle/subject?id=${encodeURIComponent(subject.subject_id)}`, {
      credentials: 'include',
    });
    if (!res.ok) throw new Error(await res.text());
    detail.value = await res.json();
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

function when(unix) {
  if (!unix) return '';
  return new Date(Number(unix) * 1000).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' });
}
</script>
