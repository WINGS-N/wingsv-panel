<template>
  <section class="surface-card">
    <h2 class="section-title">Серверы и трафик</h2>
    <p class="body-copy">Локальные ноды панели: статус, трафик и активные соединения.</p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <SamsungSectionLoader v-else-if="!statsReady" />
    <div v-if="statsReady" class="admin-stats">
      <div class="stat">
        <span class="stat-kicker">Текущая скорость</span>
        <span class="stat-value">
          {{ formatBytes((liveTotals.cur_rx_rate || 0) + (liveTotals.cur_tx_rate || 0)) }}/s
        </span>
        <span class="stat-meta">
          ↓ {{ formatBytes(liveTotals.cur_rx_rate || 0) }}/s · ↑ {{ formatBytes(liveTotals.cur_tx_rate || 0) }}/s
        </span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Активные сессии</span>
        <span class="stat-value">{{ liveTotals.active_sessions }}</span>
        <span class="stat-meta">{{ liveTotals.active_streams }} потоков</span>
      </div>
      <div class="stat">
        <span class="stat-kicker" title="Пир — это WireGuard-подключение клиента, заведённое на ноде"> Пиры </span>
        <span class="stat-value">{{ traffic?.totals?.peer_count ?? '—' }}</span>
        <span class="stat-meta">WG-конфиги клиентов на {{ traffic?.totals?.nodes ?? nodes.length }} нодах</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Ноды онлайн</span>
        <span class="stat-value"
          >{{ traffic?.totals?.nodes_online ?? '—' }} / {{ traffic?.totals?.nodes ?? nodes.length }}</span
        >
        <span class="stat-meta">режим: {{ traffic?.mode || '—' }}</span>
      </div>
    </div>
  </section>

  <SamsungCard class="mt-6" title="Трафик" subtitle="Приём и передача по нодам.">
    <template #actions>
      <div class="traffic-controls">
        <div class="control-group">
          <span class="control-label">Окно</span>
          <OneuiRadioGroup
            v-model="trafficRange"
            :options="rangeOptions"
            variant="pill"
            @update:model-value="loadTraffic"
          />
        </div>
        <div class="control-group">
          <span class="control-label">Интервал</span>
          <OneuiRadioGroup
            v-model="trafficSample"
            :options="sampleOptions"
            variant="pill"
            @update:model-value="loadTraffic"
          />
        </div>
      </div>
    </template>
    <div v-if="traffic" class="traffic-periods mt-4">
      <div v-for="p in periods" :key="p.key" class="traffic-period">
        <span class="traffic-period-label">{{ p.label }}</span>
        <span class="traffic-period-value">{{ formatBytes(p.rx + p.tx) }}</span>
        <span class="traffic-period-meta"
          ><span class="traffic-rx">↓ {{ formatBytes(p.rx) }}</span> ·
          <span class="traffic-tx">↑ {{ formatBytes(p.tx) }}</span></span
        >
      </div>
    </div>
    <div class="traffic-chart-wrap mt-4">
      <TrafficChart :series="traffic?.series || []" :class="{ 'is-loading': trafficLoading }" />
      <SamsungSectionLoader v-if="trafficLoading" class="traffic-chart-loader" />
    </div>
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
          <router-link class="node-link" :to="{ name: 'owner-node-detail', params: { id: n.id } }">
            <strong class="session-row-actor">{{ n.name || n.id }}</strong>
          </router-link>
          <SamsungPill :variant="n.status === 'online' ? 'online' : 'offline'" class="ml-2">
            {{ n.status }}
          </SamsungPill>
          <SamsungPill v-if="!n.local" variant="neutral" class="ml-2">
            {{ n.owner_name || 'admin' }}
          </SamsungPill>
          <span class="admin-muted ml-2">{{ nodeKindLabel(n.kind) }}</span>
          <div v-if="n.kind === 'xui' || (n.kind === 'vk_turn_proxy' && n.relay_version)" class="node-version-row">
            <SamsungPill v-if="n.kind === 'xui'" variant="info"> xray: {{ xrayLabel(n) }} </SamsungPill>
            <SamsungPill v-if="n.kind === 'vk_turn_proxy' && n.relay_version" variant="info">
              relay: {{ n.relay_version }}
            </SamsungPill>
          </div>
          <div class="node-id-row">
            <span class="admin-muted node-id-label">ID ноды:</span>
            <code class="admin-mono node-id" :title="'Скопировать ' + n.id" @click="copyId(n.id)">{{ n.id }}</code>
            <span v-if="copiedId === n.id" class="node-id-copied">скопировано</span>
          </div>
        </div>
        <div class="session-row-meta node-row-tail">
          <span>{{ n.grpc_endpoint }} · {{ n.peer_count }} пиров · {{ n.active_sessions }} сессий</span>
          <SamsungIconButton variant="secondary" size="small" aria-label="Команда подключения" @click="openConnect(n)">
            <Terminal class="button-icon" aria-hidden="true" />
          </SamsungIconButton>
          <SamsungIconButton
            v-if="n.kind === 'vk_turn_proxy'"
            variant="secondary"
            size="small"
            aria-label="Настроить wg-бэкенд"
            @click="openEdit(n)"
          >
            <Pencil class="button-icon" aria-hidden="true" />
          </SamsungIconButton>
          <SamsungIconButton variant="danger" size="small" aria-label="Удалить" @click="deleteNode(n)">
            <Trash2 class="button-icon" aria-hidden="true" />
          </SamsungIconButton>
        </div>
      </li>
      <li v-if="!nodes.length" class="session-row"><span class="admin-muted">Нод пока нет.</span></li>
    </ul>
  </SamsungCard>

  <SamsungCard
    collapsible
    default-collapsed
    class="mt-6"
    title="Граф потоков"
    subtitle="Клиент → реле → назначение, толщина = объём."
  >
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
      :flows="graphFlows"
      :node-names="nodeNames"
      :client-names="clientNames"
      :mode="flowMode"
      :live-rate="liveRate"
      class="mt-4"
    />
  </SamsungCard>

  <SamsungCard
    collapsible
    default-collapsed
    class="mt-6"
    title="Журнал соединений"
    subtitle="Недавние соединения через ноды (хранятся час)."
  >
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

  <SamsungModal
    :model-value="showAdd"
    :busy="adding"
    :title="editingId ? 'Настройка ноды' : 'Новый сервер'"
    @update:model-value="closeAdd"
  >
    <p class="body-copy">Локальная нода панели — VK TURN relay или 3x-ui. Панель опрашивает её по gRPC.</p>
    <template v-if="!connectCommand && !editingId">
      <label class="field-label mt-4">Способ подключения</label>
      <OneuiRadioGroup v-model="addMode" :options="addModeOptions" variant="pill" />
    </template>
    <label class="field-label mt-4">Тип сервера</label>
    <OneuiRadioGroup v-model="form.kind" :options="kindOptions" variant="pill" :disabled="!!editingId" />
    <OneuiInput v-model.trim="form.name" label="Название" placeholder="Мой сервер" class="mt-4" />
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
      v-model.trim="form.token"
      label="Токен (bearer)"
      placeholder="опционально"
      class="mt-3"
    />
    <p v-if="addMode === 'command' && !connectCommand" class="admin-muted mt-2">
      Токен сгенерируется автоматически. После создания скопируйте команду и выполните её на хосте ноды — она
      подключится к панели сама.
    </p>

    <template v-if="form.kind === 'vk_turn_proxy' && !connectCommand">
      <label class="field-label mt-4">WG-бэкенд провижна</label>
      <OneuiRadioGroup v-model="form.wg_backend" :options="wgBackendOptions" variant="pill" />
      <p class="admin-muted remote-toggle-hint">
        Как нода выдаёт wg-конфиг managed-клиенту: своя wg на ноде или клиент на инбаунде 3x-ui.
      </p>
      <template v-if="form.wg_backend === 'xui'">
        <label class="field-label mt-3">3x-ui нода</label>
        <OneuiSelect :model-value="form.xui_node_id" :options="xuiNodeOptions" @change="onXuiNodeChange" />
        <label class="field-label mt-3">Инбаунд</label>
        <OneuiSelect
          :model-value="form.xui_inbound_tag"
          :options="inboundOptions"
          :disabled="inboundsLoading"
          @change="form.xui_inbound_tag = $event"
        />
        <p v-if="inboundsError" class="admin-muted mt-1">{{ inboundsError }}</p>
      </template>
    </template>

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
      <SamsungButton v-if="!connectCommand" :busy="adding" @click="submitNode">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        {{ editingId ? 'Сохранить' : addMode === 'command' ? 'Создать и получить команду' : 'Создать' }}
      </SamsungButton>
      <SamsungButton variant="secondary" :disabled="adding" @click="closeAdd">
        {{ connectCommand ? 'Готово' : 'Отмена' }}
      </SamsungButton>
    </template>
  </SamsungModal>

  <SamsungModal :model-value="showConnect" title="Команда подключения ноды" @update:model-value="showConnect = false">
    <p class="body-copy">Выполните на хосте ноды — включит gRPC-управление и DTLS-provisioning.</p>
    <p v-if="connectError" class="state-error mt-2">{{ connectError }}</p>
    <SamsungSectionLoader v-else-if="!connectCommand" />
    <div v-else class="connect-block mt-4">
      <pre class="connect-cmd">{{ connectCommand }}</pre>
      <SamsungButton variant="secondary" size="small" @click="copyConnect">
        {{ connectCopied ? 'Скопировано' : 'Скопировать' }}
      </SamsungButton>
    </div>
    <template #actions>
      <SamsungButton variant="secondary" @click="showConnect = false">Готово</SamsungButton>
    </template>
  </SamsungModal>

  <WgPeers api-base="/api/owner" />
  <VkLinksCard />
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { Pencil, Plus, Terminal, Trash2 } from 'lucide-vue-next';
import WgPeers from '@/views/shared/WgPeers.vue';
import VkLinksCard from '@/components/domain/VkLinksCard.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungIconButton from '@/components/layout/SamsungIconButton.vue';
import SamsungModal from '@/components/layout/SamsungModal.vue';
import SamsungPill from '@/components/layout/SamsungPill.vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import OneuiRadioGroup from '@/components/controls/OneuiRadioGroup.vue';
import OneuiSelect from '@/components/controls/OneuiSelect.vue';
import TrafficChart from '@/components/domain/TrafficChart.vue';
import FlowGraph from '@/components/domain/FlowGraph.vue';
import { connectAdminSocket } from '@/stores/admin-socket.js';
import { formatBytes, formatUnix } from '@/utils/format.js';

const traffic = ref(null);
const nodes = ref([]);
// Live per-node stats pushed over the admin WS (node_stats events). Overlays the
// speed/session/stream tiles so they update ~1s without a full traffic refetch.
const liveByNode = ref({});
const liveTotals = computed(() => {
  const base = (traffic.value && traffic.value.totals) || {};
  const now = Date.now();
  let rx = 0,
    tx = 0,
    sessions = 0,
    streams = 0,
    any = false;
  for (const n of nodes.value) {
    const l = liveByNode.value[n.id];
    if (!l || now - l.at > 10000) continue;
    any = true;
    rx += l.rx_rate || 0;
    tx += l.tx_rate || 0;
    sessions += l.active_sessions || 0;
    streams += l.active_streams || 0;
  }
  if (!any) return base;
  return { ...base, cur_rx_rate: rx, cur_tx_rate: tx, active_sessions: sessions, active_streams: streams };
});
// The stats tiles show as soon as EITHER the REST snapshot or the first live WS
// push arrives, so the loader does not block while the socket is already feeding
// data. DB-only fields (peer count, nodes) fill in once loadAll returns.
const statsReady = computed(() => !!traffic.value || Object.keys(liveByNode.value).length > 0);
// Live flows pushed per node over the WS. Once any arrive, the graph renders from
// them (fresh ~1s); until then it uses the initial REST-loaded flows.
const flowsLive = ref({});
const graphFlows = computed(() => {
  if (flowMode.value !== 'live') return flows.value;
  const now = Date.now();
  const merged = [];
  let any = false;
  for (const n of nodes.value) {
    const l = flowsLive.value[n.id];
    if (!l || now - l.at > 10000) continue;
    any = true;
    for (const f of l.flows) merged.push(f);
  }
  return any ? merged : flows.value;
});
const flows = ref([]);
const clientNames = ref({});
const flowMode = ref('live');
const flowWindow = ref('1h');
const flowModeOptions = [
  { value: 'live', label: 'Живой' },
  { value: 'historical', label: 'История' },
];
const flowWindowOptions = [
  { value: '15m', label: '15м' },
  { value: '30m', label: '30м' },
  { value: '1h', label: '1ч' },
];
const connections = ref([]);
const connTotal = ref(0);
const connOffset = ref(0);
const connLimit = 5;
const loadError = ref('');
const addError = ref('');
const adding = ref(false);
const showAdd = ref(false);
const connectCommand = ref('');
const editingId = ref('');
const showConnect = ref(false);
const connectError = ref('');
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
// A fresh form gets a random gRPC port from a high, unprivileged range unlikely
// to clash with common services, so two nodes on one host don't collide by default.
function randomPort() {
  return 20000 + Math.floor(Math.random() * 25000);
}
const form = reactive({
  kind: 'vk_turn_proxy',
  name: '',
  host: '',
  port: randomPort(),
  token: '',
  wg_backend: '',
  xui_node_id: '',
  xui_inbound_tag: '',
});
const defaultPort = computed(() => form.port);

const wgBackendOptions = [
  // The empty value does NOT mean "own wg": it falls through to the panel-global
  // legacy provider (the XUI_WG_* env), which mints the peer on whatever 3x-ui node
  // that env names. Labelling it "own wg" sent operators' nodes to the wrong place.
  { value: '', label: 'Глобальный по умолчанию (legacy XUI_WG_*)' },
  { value: 'own', label: 'Своя wg на ноде' },
  { value: 'xui', label: 'Клиент на 3x-ui инбаунде' },
];
// 3x-ui nodes the wg backend can target, plus a "Не выбран" placeholder.
const xuiNodeOptions = computed(() => [
  { value: '', label: 'Не выбран' },
  ...nodes.value
    .filter((n) => n.kind === 'xui')
    .map((n) => ({ value: n.id, label: n.name || n.grpc_endpoint || n.id })),
]);
const inboundOptions = ref([{ value: '', label: 'Не выбран' }]);
const inboundsError = ref('');
const inboundsLoading = ref(false);

async function onXuiNodeChange(nodeId) {
  form.xui_node_id = nodeId || '';
  form.xui_inbound_tag = '';
  inboundsError.value = '';
  if (!form.xui_node_id) {
    inboundOptions.value = [{ value: '', label: 'Не выбран' }];
    return;
  }
  const reqNode = form.xui_node_id;
  inboundsLoading.value = true;
  inboundOptions.value = [{ value: '', label: 'Загрузка…' }];
  try {
    const res = await fetch(`/api/owner/nodes/${reqNode}/inbounds`, { credentials: 'include' });
    if (!res.ok) throw new Error((await res.json().catch(() => ({}))).message || 'Не удалось получить инбаунды');
    const data = await res.json();
    if (form.xui_node_id !== reqNode) return; // selection changed while loading
    inboundOptions.value = [
      { value: '', label: 'Не выбран' },
      ...(data.inbounds || []).map((i) => ({
        value: i.tag,
        label: `${i.tag}${i.remark ? ' (' + i.remark + ')' : ''} · ${i.protocol}:${i.port}`,
      })),
    ];
  } catch (err) {
    if (form.xui_node_id === reqNode) {
      inboundOptions.value = [{ value: '', label: 'Не выбран' }];
      inboundsError.value = err.message || 'Ошибка загрузки инбаундов';
    }
  } finally {
    if (form.xui_node_id === reqNode) inboundsLoading.value = false;
  }
}

function nodeKindLabel(kind) {
  return kind === 'xui' ? '3x-ui' : 'VK TURN';
}

function xrayRunning(n) {
  return (n.xray_state || '').toLowerCase() === 'running';
}
function xrayLabel(n) {
  const st = (n.xray_state || '').toLowerCase();
  const s = st === 'running' ? 'работает' : st || 'нет данных';
  return n.xray_version ? `${s} ${n.xray_version}` : s;
}

const nodeNames = computed(() => Object.fromEntries(nodes.value.map((n) => [n.id, n.name || n.id])));

// Recent average throughput (bytes/sec) over the last hour, for the flow graph's
// live header when the relay reports no per-flow rate.
const liveRate = computed(() => {
  const t = traffic.value?.totals || {};
  const cur = (liveTotals.value.cur_rx_rate || 0) + (liveTotals.value.cur_tx_rate || 0);
  if (cur > 0) return cur;
  return Math.round(((t.rx_1h || 0) + (t.tx_1h || 0)) / 3600);
});

const trafficRange = ref('24h');
const rangeOptions = [
  { value: '24h', label: '24ч' },
  { value: '7d', label: '7д' },
  { value: 'month', label: 'месяц' },
];
const trafficSample = ref('1m');
const sampleOptions = [
  { value: '1m', label: '1м' },
  { value: '15m', label: '15м' },
  { value: '30m', label: '30м' },
  { value: '1h', label: '1ч' },
];
const trafficLoading = ref(false);

// Changing the range/sample reloads only the chart (not the whole page), keeping
// the previous chart dimmed under a loader instead of blanking it.
async function loadTraffic() {
  trafficLoading.value = true;
  try {
    traffic.value = await fetchJSON(
      `/api/owner/stats/traffic?range=${trafficRange.value}&sample=${trafficSample.value}`,
    );
  } catch (err) {
    loadError.value = err.message || 'Не удалось загрузить трафик';
  } finally {
    trafficLoading.value = false;
  }
}
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
      fetchJSON(`/api/owner/stats/traffic?range=${trafficRange.value}&sample=${trafficSample.value}`),
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
  editingId.value = '';
  addMode.value = 'command';
  form.kind = 'vk_turn_proxy';
  form.name = '';
  form.host = '';
  form.token = '';
  form.port = randomPort();
  form.wg_backend = '';
  form.xui_node_id = '';
  form.xui_inbound_tag = '';
  inboundOptions.value = [{ value: '', label: 'Не выбран' }];
  showAdd.value = true;
}

async function openEdit(node) {
  addError.value = '';
  connectCommand.value = '';
  addMode.value = 'manual';
  editingId.value = node.id;
  form.kind = node.kind;
  form.name = node.name || '';
  const [host, port] = String(node.grpc_endpoint || '').split(':');
  form.host = host || '';
  form.port = Number(port) || defaultPort.value;
  form.token = '';
  form.wg_backend = node.wg_backend || '';
  form.xui_node_id = node.xui_node_id || '';
  form.xui_inbound_tag = node.xui_inbound_tag || '';
  inboundOptions.value = [{ value: '', label: 'Не выбран' }];
  showAdd.value = true;
  if (form.wg_backend === 'xui' && form.xui_node_id) {
    const keepTag = node.xui_inbound_tag || '';
    await onXuiNodeChange(form.xui_node_id);
    form.xui_inbound_tag = keepTag;
  }
}

async function submitNode() {
  if (editingId.value) {
    await saveEdit();
  } else {
    await addNode();
  }
}

async function saveEdit() {
  addError.value = '';
  const host = form.host.trim();
  const port = Number(form.port) || defaultPort.value;
  adding.value = true;
  try {
    const res = await fetch(`/api/owner/nodes/${editingId.value}`, {
      method: 'PUT',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: form.name.trim() || host,
        grpc_endpoint: host ? `${host}:${port}` : '',
        grpc_token: form.token.trim(),
        wg_backend: form.kind === 'vk_turn_proxy' ? form.wg_backend : '',
        xui_node_id: form.wg_backend === 'xui' ? form.xui_node_id : '',
        xui_inbound_tag: form.wg_backend === 'xui' ? form.xui_inbound_tag : '',
      }),
    });
    if (!res.ok) throw new Error((await res.json().catch(() => ({}))).message || 'Не удалось сохранить');
    closeAdd();
    await loadAll();
  } catch (err) {
    addError.value = err.message;
  } finally {
    adding.value = false;
  }
}

function closeAdd() {
  showAdd.value = false;
  connectCommand.value = '';
  editingId.value = '';
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
        wg_backend: form.kind === 'vk_turn_proxy' ? form.wg_backend : '',
        xui_node_id: form.wg_backend === 'xui' ? form.xui_node_id : '',
        xui_inbound_tag: form.wg_backend === 'xui' ? form.xui_inbound_tag : '',
      }),
    });
    if (!res.ok) throw new Error(await res.text());
    const created = await res.json();
    connectCommand.value = buildConnectCommand(created.id, form.kind, form.token.trim(), created.grpc_endpoint);
    await loadAll();
  } catch (err) {
    addError.value = err.message || 'Не удалось добавить';
  } finally {
    adding.value = false;
  }
}

// The relay must listen on exactly the port this node's endpoint names, so the
// command carries it rather than letting connect.sh fall back to its own default
// and leave the node unreachable for the collector.
function connectPortEnv(endpoint) {
  const port = String(endpoint || '')
    .split(':')
    .pop();
  return /^\d+$/.test(port) ? `VKTP_GRPC_PORT=${port} ` : '';
}

function buildConnectCommand(nodeId, kind, token, endpoint) {
  const origin = window.location.origin;
  const grpc = `${window.location.hostname}:443`;
  const k = kind === 'xui' ? 'xui' : 'vktp';
  const tok = token || '<token>';
  const env = connectPortEnv(endpoint);
  return `curl -fsSL ${origin}/connect.sh | ${env}sh -s -- grpc connect ${grpc} ${tok} ${nodeId} ${k}`;
}

async function openConnect(node) {
  showConnect.value = true;
  connectError.value = '';
  connectCommand.value = '';
  try {
    const res = await fetch(`/api/owner/nodes/${node.id}/connect`, { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    if (!data.grpc_token) {
      connectError.value =
        'У ноды нет токена в панели. Задайте его через редактирование ноды, затем откройте команду снова.';
      return;
    }
    connectCommand.value = buildConnectCommand(data.id, data.kind, data.grpc_token, data.grpc_endpoint);
  } catch (err) {
    connectError.value = err.message || 'Не удалось получить команду';
  }
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
  // Slow full refresh for the DB-backed parts (chart, totals, peer counts, node
  // status, connection log). The live tiles and flow graph update over the WS, so
  // this no longer needs the old 3s cadence that spammed the REST API.
  timer = setInterval(loadAll, 20000);
  socketHandle = connectAdminSocket((event) => {
    if (event.kind === 'node_stats' && event.payload && event.payload.node_id) {
      liveByNode.value = { ...liveByNode.value, [event.payload.node_id]: { ...event.payload, at: Date.now() } };
    } else if (event.kind === 'node_flows' && event.payload && event.payload.node_id) {
      flowsLive.value = {
        ...flowsLive.value,
        [event.payload.node_id]: { flows: event.payload.flows || [], at: Date.now() },
      };
    }
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
.node-version-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
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
.traffic-controls {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
}
.control-group {
  display: flex;
  align-items: center;
  gap: 8px;
}
.control-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--wings-kicker, #8b95a5);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}
.traffic-chart-wrap {
  position: relative;
}
.traffic-chart-wrap .is-loading {
  opacity: 0.3;
  transition: opacity 0.2s ease;
}
.traffic-chart-loader {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
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
