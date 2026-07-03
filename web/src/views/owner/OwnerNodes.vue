<template>
  <section class="surface-card">
    <h2 class="section-title">Серверы и трафик</h2>
    <p class="body-copy">Локальные ноды панели: статус, трафик и активные соединения.</p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <SamsungSectionLoader v-else-if="!traffic" />
    <div v-if="traffic" class="admin-stats">
      <div class="stat">
        <span class="stat-kicker">Трафик за 24ч</span>
        <span class="stat-value">{{ formatBytes(traffic.totals.rx_24h + traffic.totals.tx_24h) }}</span>
        <span class="stat-meta">
          ↓ {{ formatBytes(traffic.totals.rx_24h) }} · ↑ {{ formatBytes(traffic.totals.tx_24h) }}
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
        <span class="stat-meta">на {{ traffic.totals.nodes }} нодах</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Ноды онлайн</span>
        <span class="stat-value">{{ traffic.totals.nodes_online }} / {{ traffic.totals.nodes }}</span>
        <span class="stat-meta">режим: {{ traffic.mode || '—' }}</span>
      </div>
    </div>
  </section>

  <SamsungCard class="mt-6" title="Трафик" subtitle="Приём и передача за последние 24 часа.">
    <TrafficSparkline :series="traffic?.series || []" class="mt-4" />
  </SamsungCard>

  <SamsungCard class="mt-6" title="Ноды" subtitle="Все управляемые серверы и их статус.">
    <ul class="admin-list mt-4">
      <li v-for="n in nodes" :key="n.id" class="session-row">
        <div>
          <strong class="session-row-actor">{{ n.name || n.id }}</strong>
          <SamsungPill :variant="n.status === 'online' ? 'online' : 'offline'" class="ml-2">
            {{ n.status }}
          </SamsungPill>
          <SamsungPill v-if="!n.local" variant="neutral" class="ml-2">
            {{ n.owner_name || 'admin' }}
          </SamsungPill>
        </div>
        <span class="session-row-meta">
          {{ n.grpc_endpoint }} · {{ n.peer_count }} пиров · {{ n.active_sessions }} сессий
        </span>
      </li>
      <li v-if="!nodes.length" class="session-row"><span class="admin-muted">Нод пока нет.</span></li>
    </ul>
  </SamsungCard>

  <SamsungCard class="mt-6" title="Цепочки соединений" subtitle="Клиент → поток → назначение.">
    <FlowChain :flows="flows" class="mt-4" />
  </SamsungCard>

  <SamsungCard class="mt-6" title="Журнал соединений" subtitle="Недавние соединения через ноды.">
    <ul class="admin-list mt-4">
      <li v-for="c in connections" :key="c.node_id + c.session_id + c.stream_id + c.first_seen" class="session-row">
        <div>
          <strong class="session-row-actor">{{ c.client_ip || '—' }}</strong>
          <code class="admin-mono ml-2">{{ c.protocol || '—' }}</code>
          <span class="admin-muted ml-2">→ {{ c.remote || '—' }}</span>
        </div>
        <span class="session-row-meta">{{ formatBytes(c.rx_bytes + c.tx_bytes) }} · {{ formatTs(c.last_seen) }}</span>
      </li>
      <li v-if="!connections.length" class="session-row">
        <span class="admin-muted">Соединений пока не было.</span>
      </li>
    </ul>
  </SamsungCard>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungPill from '@/components/layout/SamsungPill.vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';
import TrafficSparkline from '@/components/domain/TrafficSparkline.vue';
import FlowChain from '@/components/domain/FlowChain.vue';
import { connectAdminSocket } from '@/stores/admin-socket.js';
import { formatBytes, formatUnix } from '@/utils/format.js';

const traffic = ref(null);
const nodes = ref([]);
const flows = ref([]);
const connections = ref([]);
const loadError = ref('');

let timer = null;
let socketHandle = null;

const formatTs = formatUnix;

async function fetchJSON(path) {
  const res = await fetch(path, { credentials: 'include' });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

async function loadAll() {
  try {
    const [t, n, f, c] = await Promise.all([
      fetchJSON('/api/owner/stats/traffic'),
      fetchJSON('/api/owner/nodes'),
      fetchJSON('/api/owner/stats/flows'),
      fetchJSON('/api/owner/stats/connections?limit=50'),
    ]);
    traffic.value = t;
    nodes.value = n.nodes || [];
    flows.value = f.flows || [];
    connections.value = c.connections || [];
    loadError.value = '';
  } catch (err) {
    loadError.value = err.message || 'Не удалось загрузить статистику';
  }
}

onMounted(() => {
  loadAll();
  timer = setInterval(loadAll, 5000);
  socketHandle = connectAdminSocket((event) => {
    if (event.kind === 'stats_update') loadAll();
  });
});

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
  if (socketHandle) socketHandle.close();
});
</script>
