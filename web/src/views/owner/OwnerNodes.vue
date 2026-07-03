<template>
  <section class="surface-card">
    <h2 class="section-title">Серверы и трафик</h2>
    <p class="body-copy">Локальные ноды панели: статус, трафик и активные соединения.</p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <SamsungSectionLoader v-else-if="!traffic" />
    <div v-if="traffic" class="admin-stats">
      <div class="stat">
        <span class="stat-kicker">Текущая скорость</span>
        <span class="stat-value">
          {{ formatBytes((traffic.totals.cur_rx_rate || 0) + (traffic.totals.cur_tx_rate || 0)) }}/s
        </span>
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
        <span class="stat-kicker" title="Пир — это WireGuard-подключение клиента, заведённое на ноде"> Пиры </span>
        <span class="stat-value">{{ traffic.totals.peer_count }}</span>
        <span class="stat-meta">WG-конфиги клиентов на {{ traffic.totals.nodes }} нодах</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Ноды онлайн</span>
        <span class="stat-value">{{ traffic.totals.nodes_online }} / {{ traffic.totals.nodes }}</span>
        <span class="stat-meta">режим: {{ traffic.mode || '—' }}</span>
      </div>
    </div>
  </section>

  <SamsungCard class="mt-6" title="Трафик" subtitle="Приём и передача по нодам.">
    <template #actions>
      <OneuiRadioGroup v-model="trafficRange" :options="rangeOptions" variant="pill" @update:model-value="loadAll" />
    </template>
    <div v-if="traffic" class="traffic-periods mt-4">
      <div v-for="p in periods" :key="p.key" class="traffic-period">
        <span class="traffic-period-label">{{ p.label }}</span>
        <span class="traffic-period-value">{{ formatBytes(p.rx + p.tx) }}</span>
        <span class="traffic-period-meta">↓ {{ formatBytes(p.rx) }} · ↑ {{ formatBytes(p.tx) }}</span>
      </div>
    </div>
    <TrafficChart :series="traffic?.series || []" class="mt-4" />
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
          <div class="node-id-row">
            <span class="admin-muted node-id-label">ID ноды:</span>
            <code class="admin-mono node-id" :title="'Скопировать ' + n.id" @click="copyId(n.id)">{{ n.id }}</code>
            <span v-if="copiedId === n.id" class="node-id-copied">скопировано</span>
          </div>
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

  <SamsungCard class="mt-6" title="Граф потоков" subtitle="Клиент → реле → назначение, толщина = объём.">
    <template #actions>
      <div class="flow-controls">
        <OneuiRadioGroup v-model="flowMode" :options="flowModeOptions" variant="pill" @update:model-value="loadAll" />
        <OneuiRadioGroup
          v-if="flowMode === 'historical'"
          v-model="flowWindow"
          :options="flowWindowOptions"
          variant="pill"
          @update:model-value="loadAll"
        />
      </div>
    </template>
    <FlowGraph
      :flows="flows"
      :node-names="nodeNames"
      :client-names="clientNames"
      :mode="flowMode"
      :live-rate="liveRate"
      class="mt-4"
    />
  </SamsungCard>

  <SamsungCard class="mt-6" title="Журнал соединений" subtitle="Недавние соединения через ноды (хранятся сутки).">
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
    <div v-if="connTotal > connLimit" class="conn-pager mt-4">
      <SamsungButton variant="secondary" size="small" :disabled="connOffset === 0" @click="pageConns(-1)">
        Назад
      </SamsungButton>
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

  <SamsungModal :model-value="showAdd" :busy="adding" title="Новый сервер" @update:model-value="closeAdd">
    <p class="body-copy">Локальная нода панели — VK TURN relay или 3x-ui. Панель опрашивает её по gRPC.</p>
    <template v-if="!connectCommand">
      <label class="field-label mt-4">Способ подключения</label>
      <OneuiRadioGroup v-model="addMode" :options="addModeOptions" variant="pill" />
    </template>
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
    <OneuiInput
      v-if="addMode === 'manual'"
      v-model.trim="form.token"
      label="Токен (bearer)"
      placeholder="опционально"
      class="mt-3"
    />
    <p v-if="addMode === 'command' && !connectCommand" class="admin-muted mt-2">
      Токен сгенерируется автоматически. После создания скопируйте команду и выполните её на хосте ноды — она
      подключится к панели сама.
    </p>
    <p v-if="addError" class="state-error mt-2">{{ addError }}</p>

    <div v-if="connectCommand" class="connect-block mt-4">
      <label class="field-label">Команда подключения ноды</label>
      <p class="admin-muted connect-hint">Выполните на хосте ноды — включит gRPC-управление и DTLS-provisioning.</p>
      <pre class="connect-cmd">{{ connectCommand }}</pre>
      <SamsungButton variant="secondary" size="small" @click="copyConnect">
        {{ connectCopied ? 'Скопировано' : 'Скопировать' }}
      </SamsungButton>
    </div>

    <template #actions>
      <SamsungButton v-if="!connectCommand" :busy="adding" @click="addNode">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        {{ addMode === 'command' ? 'Создать и получить команду' : 'Создать' }}
      </SamsungButton>
      <SamsungButton variant="secondary" :disabled="adding" @click="closeAdd">
        {{ connectCommand ? 'Готово' : 'Отмена' }}
      </SamsungButton>
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
import TrafficChart from '@/components/domain/TrafficChart.vue';
import FlowGraph from '@/components/domain/FlowGraph.vue';
import { connectAdminSocket } from '@/stores/admin-socket.js';
import { formatBytes, formatUnix } from '@/utils/format.js';

const traffic = ref(null);
const nodes = ref([]);
const flows = ref([]);
const clientNames = ref({});
const flowMode = ref('live');
const flowWindow = ref('1h');
const flowModeOptions = [
  { value: 'live', label: 'Живой' },
  { value: 'historical', label: 'История' },
];
const flowWindowOptions = [
  { value: '1h', label: '1ч' },
  { value: '6h', label: '6ч' },
  { value: '24h', label: '24ч' },
];
const connections = ref([]);
const connTotal = ref(0);
const connOffset = ref(0);
const connLimit = 50;
const loadError = ref('');
const addError = ref('');
const adding = ref(false);
const showAdd = ref(false);
const connectCommand = ref('');
const addMode = ref('command');
const addModeOptions = [
  { value: 'command', label: 'Командой' },
  { value: 'manual', label: 'Вручную (gRPC)' },
];

// genToken mints a node bearer token client-side for the command flow, so the
// operator never hand-enters credentials.
function genToken() {
  const bytes = new Uint8Array(24);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

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

const nodeNames = computed(() => Object.fromEntries(nodes.value.map((n) => [n.id, n.name || n.id])));

// Recent average throughput (bytes/sec) over the last hour, for the flow graph's
// live header when the relay reports no per-flow rate.
const liveRate = computed(() => {
  const t = traffic.value?.totals || {};
  const cur = (t.cur_rx_rate || 0) + (t.cur_tx_rate || 0);
  if (cur > 0) return cur;
  return Math.round(((t.rx_1h || 0) + (t.tx_1h || 0)) / 3600);
});

const trafficRange = ref('24h');
const rangeOptions = [
  { value: '24h', label: '24ч' },
  { value: '7d', label: '7д' },
  { value: 'month', label: 'месяц' },
];
const periods = computed(() => {
  const t = traffic.value?.totals || {};
  return [
    { key: '1h', label: 'за час', rx: t.rx_1h || 0, tx: t.tx_1h || 0 },
    { key: '24h', label: 'за 24ч', rx: t.rx_24h || 0, tx: t.tx_24h || 0 },
    { key: '7d', label: 'за неделю', rx: t.rx_7d || 0, tx: t.tx_7d || 0 },
    { key: 'all', label: 'за всё время', rx: t.rx_all || 0, tx: t.tx_all || 0 },
  ];
});

const copiedId = ref('');
async function copyId(id) {
  try {
    await navigator.clipboard.writeText(id);
    copiedId.value = id;
    setTimeout(() => {
      if (copiedId.value === id) copiedId.value = '';
    }, 1500);
  } catch {
    // Clipboard denied (insecure context); the id stays visible to copy manually.
  }
}

const connRangeLabel = computed(() => {
  if (!connTotal.value) return '';
  const from = connOffset.value + 1;
  const to = Math.min(connOffset.value + connLimit, connTotal.value);
  return `${from}-${to} из ${connTotal.value}`;
});

function pageConns(dir) {
  const next = connOffset.value + dir * connLimit;
  if (next < 0 || next >= connTotal.value) return;
  connOffset.value = next;
  loadAll();
}

let timer = null;
let socketHandle = null;

const formatTs = formatUnix;

async function fetchJSON(path) {
  const res = await fetch(path, { credentials: 'include' });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

const flowPath = () =>
  flowMode.value === 'historical'
    ? `/api/owner/stats/flowhistory?window=${flowWindow.value}`
    : '/api/owner/stats/flows';

async function loadAll() {
  try {
    const [t, n, f, c] = await Promise.all([
      fetchJSON(`/api/owner/stats/traffic?range=${trafficRange.value}`),
      fetchJSON('/api/owner/nodes'),
      fetchJSON(flowPath()),
      fetchJSON(`/api/owner/stats/connections?limit=${connLimit}&offset=${connOffset.value}`),
    ]);
    traffic.value = t;
    nodes.value = n.nodes || [];
    flows.value = f.flows || [];
    clientNames.value = f.client_names || {};
    connections.value = c.connections || [];
    connTotal.value = c.total || 0;
    loadError.value = '';
  } catch (err) {
    loadError.value = err.message || 'Не удалось загрузить статистику';
  }
}

function openAdd() {
  addError.value = '';
  connectCommand.value = '';
  addMode.value = 'command';
  form.name = '';
  form.host = '';
  form.token = '';
  form.port = defaultPort.value;
  showAdd.value = true;
}

function closeAdd() {
  showAdd.value = false;
  connectCommand.value = '';
}

async function addNode() {
  addError.value = '';
  const host = form.host.trim();
  const port = Number(form.port) || defaultPort.value;
  if (!host) {
    addError.value = 'Укажите IP или домен';
    return;
  }
  if (addMode.value === 'command') {
    form.token = genToken();
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
    const created = await res.json();
    connectCommand.value = buildConnectCommand(created.id, form.kind, form.token.trim());
    await loadAll();
  } catch (err) {
    addError.value = err.message || 'Не удалось добавить';
  } finally {
    adding.value = false;
  }
}

function buildConnectCommand(nodeId, kind, token) {
  const origin = window.location.origin;
  const grpc = `${window.location.hostname}:443`;
  const k = kind === 'xui' ? 'xui' : 'vktp';
  const tok = token || '<token>';
  return `curl -fsSL ${origin}/connect.sh | sh -s -- grpc connect ${grpc} ${tok} ${nodeId} ${k}`;
}

const connectCopied = ref(false);
async function copyConnect() {
  try {
    await navigator.clipboard.writeText(connectCommand.value);
    connectCopied.value = true;
    setTimeout(() => (connectCopied.value = false), 1500);
  } catch {
    // Clipboard denied; the command stays selectable in the block.
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
  timer = setInterval(loadAll, 3000);
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
  flex-wrap: wrap;
  gap: 12px;
  align-items: flex-end;
}
.node-endpoint-host {
  flex: 3 1 240px;
  min-width: 0;
}
.node-endpoint-port {
  flex: 1 1 150px;
  min-width: 140px;
}
.node-endpoint-row :deep(.field-label) {
  white-space: nowrap;
}
.node-row-tail {
  display: flex;
  align-items: center;
  gap: 12px;
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
.node-id-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.node-id-label {
  font-size: 12px;
}
.node-id {
  cursor: pointer;
  font-size: 12px;
  padding: 1px 6px;
  border-radius: 6px;
  background: rgba(252, 252, 252, 0.06);
}
.node-id:hover {
  background: rgba(75, 141, 255, 0.18);
}
.node-id-copied {
  font-size: 12px;
  color: #5ecb9e;
}
.traffic-periods {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
}
.traffic-period {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px 12px;
  border-radius: 12px;
  background: rgba(252, 252, 252, 0.04);
}
.traffic-period-label {
  font-size: 12px;
  color: rgba(252, 252, 252, 0.55);
}
.traffic-period-value {
  font-size: 18px;
  font-weight: 600;
}
.traffic-period-meta {
  font-size: 12px;
  color: rgba(252, 252, 252, 0.5);
}
.flow-controls {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.connect-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
}
.connect-hint {
  font-size: 12px;
  margin: 0;
}
.connect-cmd {
  width: 100%;
  overflow-x: auto;
  background: rgba(252, 252, 252, 0.06);
  border-radius: 10px;
  padding: 10px 12px;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}
</style>
