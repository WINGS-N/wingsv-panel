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
    <template #actions>
      <SamsungButton variant="secondary" @click="openAdd">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        Сервер
      </SamsungButton>
    </template>
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
          <span class="admin-muted ml-2">{{ nodeKindLabel(n.kind) }}</span>
        </div>
        <div class="session-row-meta node-row-tail">
          <span>{{ n.grpc_endpoint }} · {{ n.peer_count }} пиров · {{ n.active_sessions }} сессий</span>
          <SamsungIconButton variant="danger" size="small" aria-label="Удалить" @click="deleteNode(n)">
            <Trash2 class="button-icon" aria-hidden="true" />
          </SamsungIconButton>
        </div>
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

  <SamsungModal :model-value="showAdd" :busy="adding" title="Новый сервер" @update:model-value="closeAdd">
    <p class="body-copy">Локальная нода панели — VK TURN relay или 3x-ui. Панель опрашивает её по gRPC.</p>
    <label class="field-label mt-4">Тип сервера</label>
    <OneuiRadioGroup v-model="form.kind" :options="kindOptions" variant="pill" />
    <OneuiInput v-model.trim="form.name" label="Название" placeholder="relay.example.com" class="mt-4" />
    <div class="node-endpoint-row mt-3">
      <OneuiInput v-model.trim="form.host" label="IP или домен" placeholder="relay.example.com" class="node-endpoint-host" />
      <OneuiInput
        v-model.number="form.port"
        label="gRPC порт"
        type="number"
        :placeholder="String(defaultPort)"
        class="node-endpoint-port"
      />
    </div>
    <OneuiInput v-model.trim="form.token" label="Токен (bearer)" placeholder="опционально" class="mt-3" />
    <p v-if="addError" class="state-error mt-2">{{ addError }}</p>
    <template #actions>
      <SamsungButton :busy="adding" @click="addNode">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        Создать
      </SamsungButton>
      <SamsungButton variant="secondary" :disabled="adding" @click="closeAdd">Отмена</SamsungButton>
    </template>
  </SamsungModal>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { Plus, Trash2 } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungIconButton from '@/components/layout/SamsungIconButton.vue';
import SamsungModal from '@/components/layout/SamsungModal.vue';
import SamsungPill from '@/components/layout/SamsungPill.vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import OneuiRadioGroup from '@/components/controls/OneuiRadioGroup.vue';
import TrafficSparkline from '@/components/domain/TrafficSparkline.vue';
import FlowChain from '@/components/domain/FlowChain.vue';
import { connectAdminSocket } from '@/stores/admin-socket.js';
import { formatBytes, formatUnix } from '@/utils/format.js';

const traffic = ref(null);
const nodes = ref([]);
const flows = ref([]);
const connections = ref([]);
const loadError = ref('');
const addError = ref('');
const adding = ref(false);
const showAdd = ref(false);

const kindOptions = [
  { value: 'vk_turn_proxy', label: 'VK TURN' },
  { value: 'xui', label: '3x-ui' },
];
// Default gRPC ports for a fresh form: VK TURN relay 25612, 3x-ui panel API 25613.
const defaultPorts = { vk_turn_proxy: 25612, xui: 25613 };
const form = reactive({ kind: 'vk_turn_proxy', name: '', host: '', port: 25612, token: '' });
const defaultPort = computed(() => defaultPorts[form.kind] || 25612);
watch(
  () => form.kind,
  (kind) => {
    form.port = defaultPorts[kind] || form.port;
  },
);

function nodeKindLabel(kind) {
  return kind === 'xui' ? '3x-ui' : 'VK TURN';
}

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

function openAdd() {
  addError.value = '';
  form.name = '';
  form.host = '';
  form.token = '';
  form.port = defaultPort.value;
  showAdd.value = true;
}

function closeAdd() {
  showAdd.value = false;
}

async function addNode() {
  addError.value = '';
  const host = form.host.trim();
  const port = Number(form.port) || defaultPort.value;
  if (!host) {
    addError.value = 'Укажите IP или домен';
    return;
  }
  adding.value = true;
  try {
    const res = await fetch('/api/owner/nodes', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        kind: form.kind,
        name: form.name.trim() || host,
        grpc_endpoint: `${host}:${port}`,
        grpc_token: form.token.trim(),
      }),
    });
    if (!res.ok) throw new Error(await res.text());
    showAdd.value = false;
    await loadAll();
  } catch (err) {
    addError.value = err.message || 'Не удалось добавить';
  } finally {
    adding.value = false;
  }
}

async function deleteNode(node) {
  try {
    const res = await fetch(`/api/owner/nodes/${node.id}`, { method: 'DELETE', credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    await loadAll();
  } catch (err) {
    loadError.value = err.message || 'Не удалось удалить';
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

<style scoped>
.node-endpoint-row {
  display: flex;
  gap: 12px;
  align-items: flex-end;
}
.node-endpoint-host {
  flex: 1 1 auto;
}
.node-endpoint-port {
  flex: 0 0 130px;
}
.node-row-tail {
  display: flex;
  align-items: center;
  gap: 12px;
}
</style>
