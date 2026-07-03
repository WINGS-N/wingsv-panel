<template>
  <section class="surface-card">
    <h2 class="section-title">Мои серверы</h2>
    <p class="body-copy">
      Подключите свои внешние VK TURN / 3x-ui эндпоинты по gRPC, чтобы видеть их статистику и трафик.
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>

    <p v-if="!allowGRPC" class="admin-muted mt-4">
      Owner пока не разрешил администраторам подключать свои gRPC-эндпоинты.
    </p>

    <div v-if="allowGRPC" class="actions-row mt-4">
      <SamsungButton @click="openAdd">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        Добавить сервер
      </SamsungButton>
    </div>
  </section>

  <SamsungModal :model-value="showAdd" :busy="adding" title="Новый сервер" @update:model-value="closeAdd">
    <p class="body-copy">Ваш внешний VK TURN relay или 3x-ui сервер — панель будет опрашивать его по gRPC.</p>
    <template v-if="!connectCommand">
      <label class="field-label mt-4">Способ подключения</label>
      <OneuiRadioGroup v-model="addMode" :options="addModeOptions" variant="pill" />
    </template>
    <label class="field-label mt-4">Тип сервера</label>
    <OneuiRadioGroup v-model="form.kind" :options="kindOptions" variant="pill" />
    <OneuiInput v-model.trim="form.name" label="Название" placeholder="Мой релей" class="mt-4" />
    <div class="node-endpoint-row mt-3">
      <OneuiInput
        v-model.trim="form.host"
        label="IP или домен"
        placeholder="relay.example.com"
        class="node-endpoint-host"
      />
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
      v-model.trim="form.grpc_token"
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
      <SamsungButton v-if="!connectCommand" :busy="adding" @click="onAdd">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        {{ addMode === 'command' ? 'Создать и получить команду' : 'Создать' }}
      </SamsungButton>
      <SamsungButton variant="secondary" :disabled="adding" @click="closeAdd">
        {{ connectCommand ? 'Готово' : 'Отмена' }}
      </SamsungButton>
    </template>
  </SamsungModal>

  <SamsungCard v-if="nodes.length" class="mt-6" title="Ноды" subtitle="Ваши подключённые серверы.">
    <ul class="admin-list mt-4">
      <li v-for="n in nodes" :key="n.id" class="session-row">
        <div>
          <router-link class="node-link" :to="{ name: 'admin-node-detail', params: { id: n.id } }">
            <strong class="session-row-actor">{{ n.name || n.id }}</strong>
          </router-link>
          <SamsungPill :variant="n.status === 'online' ? 'online' : 'offline'" class="ml-2">
            {{ n.status }}
          </SamsungPill>
          <span class="admin-muted ml-2">{{ n.grpc_endpoint }}</span>
          <div class="node-id-row">
            <span class="admin-muted node-id-label">ID ноды:</span>
            <code class="admin-mono node-id" :title="'Скопировать ' + n.id" @click="copyId(n.id)">{{ n.id }}</code>
            <span v-if="copiedId === n.id" class="node-id-copied">скопировано</span>
          </div>
        </div>
        <div class="session-row-meta node-row-tail">
          <span>{{ n.peer_count }} пиров · {{ n.active_sessions }} сессий</span>
          <SamsungButton v-if="n.kind === 'xui'" variant="secondary" size="small" @click="openAddClient(n)">
            <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
            Клиент
          </SamsungButton>
          <SamsungIconButton variant="danger" size="small" aria-label="Удалить" @click="onDelete(n)">
            <Trash2 class="button-icon" aria-hidden="true" />
          </SamsungIconButton>
        </div>
      </li>
    </ul>
  </SamsungCard>

  <SamsungModal
    :model-value="!!clientNode"
    :busy="clientAdding"
    title="Новый inbound-клиент 3x-ui"
    @update:model-value="closeAddClient"
  >
    <p class="body-copy">
      JSON настроек клиента (в том же виде, что принимает REST 3x-ui): email, id/пароль, лимиты и т.п.
    </p>
    <OneuiTextarea v-model="clientPayload" rows="6" label="payload_json" placeholder='{"email":"user1", ...}' />
    <p v-if="clientError" class="state-error mt-2">{{ clientError }}</p>
    <template #actions>
      <SamsungButton :busy="clientAdding" @click="submitAddClient">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        Создать
      </SamsungButton>
      <SamsungButton variant="secondary" :disabled="clientAdding" @click="closeAddClient">Отмена</SamsungButton>
    </template>
  </SamsungModal>

  <template v-if="traffic && nodes.length">
    <section class="surface-card mt-6">
      <div class="admin-stats">
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

    <SamsungCard class="mt-6" title="Трафик" subtitle="Приём и передача по вашим нодам.">
      <template #actions>
        <OneuiRadioGroup
          v-model="trafficRange"
          :options="rangeOptions"
          variant="pill"
          @update:model-value="loadStats"
        />
      </template>
      <div class="traffic-periods mt-4">
        <div v-for="p in periods" :key="p.key" class="traffic-period">
          <span class="traffic-period-label">{{ p.label }}</span>
          <span class="traffic-period-value">{{ formatBytes(p.rx + p.tx) }}</span>
          <span class="traffic-period-meta">↓ {{ formatBytes(p.rx) }} · ↑ {{ formatBytes(p.tx) }}</span>
        </div>
      </div>
      <TrafficChart :series="traffic.series || []" class="mt-4" />
    </SamsungCard>

    <SamsungCard class="mt-6" title="Граф потоков" subtitle="Клиент → реле → назначение, толщина = объём.">
      <template #actions>
        <div class="flow-controls">
          <OneuiRadioGroup
            v-model="flowMode"
            :options="flowModeOptions"
            variant="pill"
            @update:model-value="loadStats"
          />
          <OneuiRadioGroup
            v-if="flowMode === 'historical'"
            v-model="flowWindow"
            :options="flowWindowOptions"
            variant="pill"
            @update:model-value="loadStats"
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

    <SamsungCard
      class="mt-6"
      title="Журнал соединений"
      subtitle="Недавние соединения через ваши ноды (хранятся сутки)."
    >
      <ul class="admin-list mt-4">
        <li v-for="c in connections" :key="c.node_id + c.session_id + c.stream_id + c.first_seen" class="session-row">
          <div>
            <strong class="session-row-actor">{{ c.client_ip || '—' }}</strong>
            <code class="admin-mono ml-2">{{ c.protocol || '—' }}</code>
            <span class="admin-muted ml-2">→ {{ c.remote || '—' }}</span>
          </div>
          <span class="session-row-meta">
            {{ formatBytes(c.rx_bytes + c.tx_bytes) }} · {{ formatTs(c.last_seen) }}
          </span>
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
  </template>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { Plus, Trash2 } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungIconButton from '@/components/layout/SamsungIconButton.vue';
import SamsungModal from '@/components/layout/SamsungModal.vue';
import SamsungPill from '@/components/layout/SamsungPill.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import OneuiRadioGroup from '@/components/controls/OneuiRadioGroup.vue';
import OneuiTextarea from '@/components/controls/OneuiTextarea.vue';
import TrafficChart from '@/components/domain/TrafficChart.vue';
import FlowGraph from '@/components/domain/FlowGraph.vue';
import { connectAdminSocket } from '@/stores/admin-socket.js';
import { formatBytes, formatUnix } from '@/utils/format.js';

const nodes = ref([]);
const allowGRPC = ref(false);
const traffic = ref(null);
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

function genToken() {
  const bytes = new Uint8Array(24);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

const clientNode = ref(null);
const clientPayload = ref('');
const clientAdding = ref(false);
const clientError = ref('');

const kindOptions = [
  { value: 'vk_turn_proxy', label: 'VK TURN' },
  { value: 'xui', label: '3x-ui' },
];
// A fresh form gets a random gRPC port from a high, unprivileged range unlikely
// to clash with common services, so two nodes on one host don't collide by default.
function randomPort() {
  return 20000 + Math.floor(Math.random() * 25000);
}
const form = reactive({ name: '', kind: 'vk_turn_proxy', host: '', port: randomPort(), grpc_token: '' });
const defaultPort = computed(() => form.port);

const formatTs = formatUnix;

const nodeNames = computed(() => Object.fromEntries(nodes.value.map((n) => [n.id, n.name || n.id])));

const liveRate = computed(() => {
  const t = traffic.value?.totals || {};
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
  loadStats();
}

let timer = null;
let socketHandle = null;

async function fetchJSON(path) {
  const res = await fetch(path, { credentials: 'include' });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

async function loadNodes() {
  try {
    const body = await fetchJSON('/api/admin/nodes');
    nodes.value = body.nodes || [];
    allowGRPC.value = !!body.allow_grpc;
    loadError.value = '';
  } catch (err) {
    loadError.value = err.message || 'Не удалось загрузить ноды';
  }
}

async function loadStats() {
  if (!nodes.value.length) {
    traffic.value = null;
    flows.value = [];
    connections.value = [];
    connTotal.value = 0;
    return;
  }
  try {
    const flowPath =
      flowMode.value === 'historical'
        ? `/api/admin/stats/flowhistory?window=${flowWindow.value}`
        : '/api/admin/stats/flows';
    const [t, f, c] = await Promise.all([
      fetchJSON(`/api/admin/stats/traffic?range=${trafficRange.value}`),
      fetchJSON(flowPath),
      fetchJSON(`/api/admin/stats/connections?limit=${connLimit}&offset=${connOffset.value}`),
    ]);
    traffic.value = t;
    flows.value = f.flows || [];
    clientNames.value = f.client_names || {};
    connections.value = c.connections || [];
    connTotal.value = c.total || 0;
  } catch (err) {
    loadError.value = err.message || 'Не удалось загрузить статистику';
  }
}

async function refresh() {
  await loadNodes();
  await loadStats();
}

function openAdd() {
  addError.value = '';
  connectCommand.value = '';
  addMode.value = 'command';
  form.name = '';
  form.host = '';
  form.grpc_token = '';
  form.port = randomPort();
  showAdd.value = true;
}

function closeAdd() {
  showAdd.value = false;
  connectCommand.value = '';
}

async function onAdd() {
  addError.value = '';
  const host = form.host.trim();
  if (!host) {
    addError.value = 'Укажите IP или домен';
    return;
  }
  const port = Number(form.port) || defaultPort.value;
  if (addMode.value === 'command') {
    form.grpc_token = genToken();
  }
  adding.value = true;
  try {
    const res = await fetch('/api/admin/nodes', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: form.name.trim() || host,
        kind: form.kind,
        grpc_endpoint: `${host}:${port}`,
        grpc_token: form.grpc_token.trim(),
      }),
    });
    if (!res.ok) throw new Error(await res.text());
    const created = await res.json();
    connectCommand.value = buildConnectCommand(created.id, form.kind, form.grpc_token.trim());
    await refresh();
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

function openAddClient(node) {
  clientNode.value = node;
  clientPayload.value = '';
  clientError.value = '';
}

function closeAddClient() {
  clientNode.value = null;
}

async function submitAddClient() {
  if (!clientNode.value) return;
  clientAdding.value = true;
  clientError.value = '';
  try {
    const res = await fetch(`/api/admin/nodes/${clientNode.value.id}/clients`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ payload_json: clientPayload.value }),
    });
    if (!res.ok) throw new Error(await res.text());
    clientNode.value = null;
  } catch (err) {
    clientError.value = err.message || 'Не удалось создать клиента';
  } finally {
    clientAdding.value = false;
  }
}

async function onDelete(node) {
  try {
    const res = await fetch(`/api/admin/nodes/${node.id}`, { method: 'DELETE', credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    await refresh();
  } catch (err) {
    loadError.value = err.message || 'Не удалось удалить';
  }
}

onMounted(() => {
  refresh();
  timer = setInterval(loadStats, 3000);
  socketHandle = connectAdminSocket((event) => {
    if (event.kind === 'stats_update') loadStats();
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
.node-link {
  text-decoration: none;
}
.node-link:hover .session-row-actor {
  color: #4b8dff;
  text-decoration: underline;
}
</style>
