<template>
  <section class="surface-card">
    <div class="federation-live-head">
      <h2 class="section-title">Точки наблюдения</h2>
      <span
        class="live-dot"
        :class="{ 'is-dead': !anyOnline }"
        role="status"
        :aria-label="anyOnline ? 'Есть точки онлайн' : 'Ни одна точка не отвечает'"
        :title="anyOnline ? 'Есть точки онлайн' : 'Ни одна точка не отвечает'"
      ></span>
    </div>
    <p class="body-copy body-copy-wide">Замеры изнутри страны: не пинг, а скачивание через настоящий профиль.</p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <div v-if="enabled" class="actions-row">
      <SamsungButton :busy="running" @click="runNow">
        <template #icon><Play class="button-icon" aria-hidden="true" /></template>
        Замерить сейчас
      </SamsungButton>
      <span v-if="runNote" class="admin-muted self-center">{{ runNote }}</span>
    </div>
    <p v-else-if="!enabled" class="state-hint">Федерация выключена.</p>
    <p v-else-if="!vantages.length" class="state-hint">Ни одна точка наблюдения ещё не выходила на связь.</p>

    <div v-if="vantages.length" class="fed-node-list mt-4">
      <div v-for="v in vantages" :key="v.probe_id" class="fed-node-row">
        <img src="/img/oneui/utilities-system-monitor.svg" alt="" class="probe-icon" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="truncate text-[17px]">{{ v.probe_id }}</span>
            <span class="admin-pill" :class="v.online ? 'is-online' : 'is-offline'">
              <span class="state-dot" :class="v.online ? 'is-live' : 'is-off'" aria-hidden="true"></span>
              {{ v.online ? 'онлайн' : 'молчит' }}
            </span>
          </div>
          <span class="mt-1 flex flex-wrap items-center gap-4 text-[13px] text-wings-muted">
            <span v-if="v.region" class="inline-flex items-center gap-1">
              <MapPin :size="13" aria-hidden="true" />{{ v.region }}
            </span>
            <span v-if="v.isp" class="inline-flex items-center gap-1">
              <Building2 :size="13" aria-hidden="true" />{{ v.isp }}
            </span>
            <span class="inline-flex items-center gap-1">
              <Activity :size="13" aria-hidden="true" />{{ v.measurements }} замеров
            </span>
            <span class="inline-flex items-center gap-1">
              <Clock :size="13" aria-hidden="true" />{{ ago(v.last_seen_unix) }}
            </span>
          </span>
        </div>
      </div>
    </div>
  </section>

  <section v-if="enabled && measurements.length" class="surface-card mt-6">
    <h2 class="section-title">Замеры</h2>
    <p class="admin-muted mt-1">
      По каждому адресу и транспорту отдельно: TCP и XHTTP режут независимо, и выдавать стоит только то, что работает.
    </p>
    <div class="fed-node-list mt-4">
      <div v-for="m in pagedMeasurements" :key="m.node_id + m.address + m.transport" class="fed-node-row">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="truncate text-[17px]">{{ m.hostname || m.node_id.slice(0, 8) }}</span>
            <span class="admin-pill">{{ m.transport }}</span>
            <span class="admin-pill" :class="m.ok ? 'is-online' : 'is-offline'">
              {{ m.ok ? 'проходит' : 'не проходит' }}
            </span>
          </div>
          <span class="mt-1 block truncate text-[13px] text-wings-muted">{{ m.address }}</span>
          <span v-if="m.error" class="mt-0.5 block truncate text-[13px] text-wings-danger">{{ m.error }}</span>
          <span class="mt-1 flex flex-wrap items-center gap-4 text-[13px] text-wings-muted">
            <span v-if="m.ok" class="inline-flex items-center gap-1">
              <Gauge :size="13" aria-hidden="true" />{{ speed(m.download_bps) }}
            </span>
            <span v-if="m.rtt_ms" class="inline-flex items-center gap-1">
              <Timer :size="13" aria-hidden="true" />{{ m.rtt_ms }} ms
            </span>
            <span v-if="m.handshake_ms" class="inline-flex items-center gap-1">
              <Handshake :size="13" aria-hidden="true" />{{ m.handshake_ms }} ms
            </span>
            <span class="inline-flex items-center gap-1">
              <Radar :size="13" aria-hidden="true" />{{ m.probe_id || 'зонд неизвестен' }}
            </span>
            <span class="inline-flex items-center gap-1">
              <Clock :size="13" aria-hidden="true" />{{ ago(m.at_unix) }}
            </span>
          </span>
        </div>
      </div>
    </div>
    <SamsungPager v-model:page="page" :total="measurements.length" :per-page="perPage" />
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { Activity, Building2, Clock, Gauge, Handshake, MapPin, Play, Radar, Timer } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungPager from '@/components/controls/SamsungPager.vue';
import { formatBytes, formatSpeed } from '@/utils/format';

const enabled = ref(false);
const vantages = ref([]);
const measurements = ref([]);
const loadError = ref('');
const running = ref(false);
const anyOnline = computed(() => vantages.value.some((v) => v.online));
const page = ref(1);
const perPage = 25;
const pagedMeasurements = computed(() => measurements.value.slice((page.value - 1) * perPage, page.value * perPage));
const runNote = ref('');
let timer = null;

onMounted(() => {
  load();
  timer = setInterval(load, 15000);
});

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});

async function load() {
  try {
    const res = await fetch('/api/owner/federation/probes', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    enabled.value = Boolean(data.enabled);
    vantages.value = data.vantages || [];
    measurements.value = data.measurements || [];
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  }
}

// Круг идёт раз в пять минут, и только что поднятую ноду иначе не проверить
async function runNow() {
  running.value = true;
  runNote.value = '';
  try {
    const res = await fetch('/api/owner/federation/probes/run', { method: 'POST', credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    runNote.value = data.probes
      ? `запущено на ${data.probes} точках, результат через минуту`
      : 'ни одна точка не онлайн';
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    running.value = false;
  }
}

// Голова присылает байты в секунду, и единица та же, что во всём остальном
function speed(bytesPerSecond) {
  const bytes = Number(bytesPerSecond) || 0;
  return formatSpeed(bytes);
}

function ago(unix) {
  const seconds = Math.max(0, Math.round(Date.now() / 1000 - Number(unix || 0)));
  if (!unix) return 'никогда';
  if (seconds < 60) return `${seconds} с назад`;
  if (seconds < 3600) return `${Math.round(seconds / 60)} мин назад`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)} ч назад`;
  return `${Math.round(seconds / 86400)} дн назад`;
}
</script>
