<template>
  <section class="surface-card">
    <router-link :to="backTo" class="node-back">← К списку серверов</router-link>
    <h2 class="section-title mt-2">{{ node?.name || nodeId }}</h2>
    <p class="body-copy">
      <SamsungPill :variant="node?.status === 'online' ? 'online' : 'offline'">{{ node?.status || '—' }}</SamsungPill>
      <span class="admin-muted ml-2">{{ node?.id }}</span>
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>

    <div v-if="traffic" class="admin-stats mt-4">
      <div class="stat">
        <span class="stat-kicker">Текущая скорость</span>
        <span class="stat-value">{{ formatBytes(curRate) }}/s</span>
        <span class="stat-meta">
          ↓ {{ formatBytes(traffic.totals.cur_rx_rate || 0) }}/s · ↑
          {{ formatBytes(traffic.totals.cur_tx_rate || 0) }}/s
        </span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Активные сессии</span>
        <span class="stat-value">{{ traffic.totals.active_sessions }}</span>
        <span class="stat-meta">{{ traffic.totals.active_streams }} потоков</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Пиры</span>
        <span class="stat-value">{{ traffic.totals.peer_count }}</span>
        <span class="stat-meta"
          >за 24ч: {{ formatBytes((traffic.totals.rx_24h || 0) + (traffic.totals.tx_24h || 0)) }}</span
        >
      </div>
    </div>
  </section>

  <SamsungCard class="mt-6" title="Трафик" subtitle="Приём и передача этой ноды.">
    <template #actions>
      <OneuiRadioGroup v-model="trafficRange" :options="rangeOptions" variant="pill" @update:model-value="load" />
    </template>
    <TrafficChart :series="traffic?.series || []" class="mt-4" />
  </SamsungCard>

  <SamsungCard class="mt-6" title="Граф потоков" subtitle="Живые потоки ноды.">
    <FlowGraph :flows="flows" :client-names="clientNames" mode="live" :live-rate="curRate" class="mt-4" />
  </SamsungCard>

  <SamsungCard class="mt-6" title="Журнал соединений" subtitle="Недавние соединения ноды (хранятся сутки).">
    <ul class="admin-list mt-4">
      <li v-for="c in connections" :key="c.session_id + c.stream_id + c.first_seen" class="session-row">
        <div>
          <strong class="session-row-actor">{{ c.client_ip || '—' }}</strong>
          <code class="admin-mono ml-2">{{ c.protocol || '—' }}</code>
          <span class="admin-muted ml-2">→ {{ c.remote || '—' }}</span>
        </div>
        <span class="session-row-meta">{{ formatBytes(c.rx_bytes + c.tx_bytes) }} · {{ formatUnix(c.last_seen) }}</span>
      </li>
      <li v-if="!connections.length" class="session-row"><span class="admin-muted">Соединений пока не было.</span></li>
    </ul>
    <div v-if="connTotal > connLimit" class="conn-pager mt-4">
      <SamsungButton variant="secondary" size="small" :disabled="connOffset === 0" @click="pageConns(-1)"
        >Назад</SamsungButton
      >
      <span class="conn-pager-info">{{ connRangeLabel }}</span>
      <SamsungButton
        variant="secondary"
        size="small"
        :disabled="connOffset + connLimit >= connTotal"
        @click="pageConns(1)"
      >
        Далее
      </SamsungButton>
    </div>
  </SamsungCard>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungPill from '@/components/layout/SamsungPill.vue';
import OneuiRadioGroup from '@/components/controls/OneuiRadioGroup.vue';
import TrafficChart from '@/components/domain/TrafficChart.vue';
import FlowGraph from '@/components/domain/FlowGraph.vue';
import { connectAdminSocket } from '@/stores/admin-socket.js';
import { formatBytes, formatUnix } from '@/utils/format.js';

const props = defineProps({
  apiBase: { type: String, default: '/api/owner' },
  backName: { type: String, default: 'owner-nodes' },
});

const route = useRoute();
const nodeId = route.params.id;

const traffic = ref(null);
const flows = ref([]);
const clientNames = ref({});
const connections = ref([]);
const connTotal = ref(0);
const connOffset = ref(0);
const connLimit = 50;
const loadError = ref('');
const trafficRange = ref('24h');
const rangeOptions = [
  { value: '24h', label: '24ч' },
  { value: '7d', label: '7д' },
  { value: 'month', label: 'месяц' },
];

const backTo = computed(() => ({ name: props.backName }));
const node = computed(() => (traffic.value?.nodes || []).find((n) => n.id === nodeId) || null);
const curRate = computed(() => {
  const t = traffic.value?.totals || {};
  return (t.cur_rx_rate || 0) + (t.cur_tx_rate || 0);
});
const connRangeLabel = computed(() => {
  if (!connTotal.value) return '';
  return `${connOffset.value + 1}-${Math.min(connOffset.value + connLimit, connTotal.value)} из ${connTotal.value}`;
});

let timer = null;
let socketHandle = null;

async function fetchJSON(path) {
  const res = await fetch(path, { credentials: 'include' });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

async function load() {
  try {
    const q = `node=${encodeURIComponent(nodeId)}`;
    const [t, f, c] = await Promise.all([
      fetchJSON(`${props.apiBase}/stats/traffic?range=${trafficRange.value}&${q}`),
      fetchJSON(`${props.apiBase}/stats/flows?${q}`),
      fetchJSON(`${props.apiBase}/stats/connections?limit=${connLimit}&offset=${connOffset.value}&${q}`),
    ]);
    traffic.value = t;
    flows.value = f.flows || [];
    clientNames.value = f.client_names || {};
    connections.value = c.connections || [];
    connTotal.value = c.total || 0;
    loadError.value = '';
  } catch (err) {
    loadError.value = err.message || 'Не удалось загрузить статистику ноды';
  }
}

function pageConns(dir) {
  const next = connOffset.value + dir * connLimit;
  if (next < 0 || next >= connTotal.value) return;
  connOffset.value = next;
  load();
}

onMounted(() => {
  load();
  timer = setInterval(load, 3000);
  socketHandle = connectAdminSocket((event) => {
    if (event.kind === 'stats_update') load();
  });
});
onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
  if (socketHandle) socketHandle.close();
});
</script>

<style scoped>
.node-back {
  color: rgba(75, 141, 255, 0.9);
  font-size: 13px;
  text-decoration: none;
}
.node-back:hover {
  text-decoration: underline;
}
.conn-pager {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 14px;
}
.conn-pager-info {
  font-size: 13px;
  color: rgba(252, 252, 252, 0.55);
}
</style>
