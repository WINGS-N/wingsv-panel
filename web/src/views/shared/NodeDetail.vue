<template>
  <section class="surface-card">
    <div class="node-detail-head">
      <div>
        <router-link :to="backTo" class="node-back">← К списку серверов</router-link>
        <h2 class="section-title mt-2">{{ meta?.name || node?.name || nodeId }}</h2>
        <p class="body-copy">
          <SamsungPill :variant="metaStatus === 'online' ? 'online' : 'offline'">{{ metaStatus || '—' }}</SamsungPill>
          <SamsungPill v-if="isXui || meta?.relay_version" variant="info" class="ml-2">
            {{ isXui ? `xray: ${meta?.xray_version || '—'}` : `relay: ${meta?.relay_version}` }}
          </SamsungPill>
          <span class="admin-muted ml-2">{{ nodeKindLabel }}</span>
          <code class="admin-mono ml-2">{{ nodeId }}</code>
        </p>
      </div>
      <div class="node-detail-actions">
        <SamsungIconButton variant="secondary" size="small" aria-label="Команда подключения" @click="openConnect">
          <Terminal class="button-icon" aria-hidden="true" />
        </SamsungIconButton>
        <SamsungIconButton
          v-if="!isXui"
          variant="secondary"
          size="small"
          aria-label="Настроить wg-бэкенд"
          @click="openEdit"
        >
          <Pencil class="button-icon" aria-hidden="true" />
        </SamsungIconButton>
        <SamsungIconButton variant="danger" size="small" aria-label="Удалить" @click="onDelete">
          <Trash2 class="button-icon" aria-hidden="true" />
        </SamsungIconButton>
      </div>
    </div>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>

    <div v-if="!isXui && statsReady" class="admin-stats mt-4">
      <div class="stat">
        <span class="stat-kicker">Текущая скорость</span>
        <span class="stat-value"
          >{{ formatBytes((liveTotals.cur_rx_rate || 0) + (liveTotals.cur_tx_rate || 0)) }}/s</span
        >
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
        <span class="stat-kicker">Пиры</span>
        <span class="stat-value">{{ traffic?.totals?.peer_count ?? '—' }}</span>
        <span class="stat-meta"
          >за 24ч: {{ formatBytes((traffic?.totals?.rx_24h || 0) + (traffic?.totals?.tx_24h || 0)) }}</span
        >
      </div>
    </div>
  </section>

  <SamsungCard
    v-if="isXui"
    collapsible
    default-collapsed
    class="mt-6"
    title="Граф потоков Xray"
    subtitle="Клиент → нода → inbound, толщина = трафик клиента."
  >
    <FlowGraph :flows="xrayFlows" :client-names="xrayNames" mode="historical" class="mt-4" />
  </SamsungCard>

  <SamsungCard v-if="!isXui" class="mt-6" title="Трафик" subtitle="Приём и передача этой ноды.">
    <template #actions>
      <OneuiRadioGroup v-model="trafficRange" :options="rangeOptions" variant="pill" @update:model-value="load" />
    </template>
    <TrafficChart :series="traffic?.series || []" class="mt-4" />
  </SamsungCard>

  <WgPeers v-if="!isXui" :api-base="apiBase" :node-id="nodeId" class="mt-6" />

  <SamsungCard
    v-if="!isXui"
    collapsible
    default-collapsed
    class="mt-6"
    title="Граф потоков"
    subtitle="Живые потоки ноды."
  >
    <FlowGraph :flows="graphFlows" :client-names="clientNames" mode="live" :live-rate="curRate" class="mt-4" />
  </SamsungCard>

  <SamsungCard
    v-if="!isXui"
    collapsible
    default-collapsed
    class="mt-6"
    title="Журнал соединений"
    subtitle="Недавние соединения ноды (хранятся час)."
  >
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

  <SamsungModal :model-value="showConnect" title="Команда подключения ноды" @update:model-value="showConnect = false">
    <p class="body-copy">Выполните на хосте ноды — включит gRPC-управление и DTLS-provisioning.</p>
    <p v-if="connectError" class="state-error mt-2">{{ connectError }}</p>
    <p v-else-if="!connectCommand" class="admin-muted mt-4">Загрузка…</p>
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

  <SamsungModal :model-value="showEdit" :busy="saving" title="Настройка ноды" @update:model-value="showEdit = false">
    <OneuiInput v-model.trim="form.name" label="Название" placeholder="Мой релей" class="mt-2" />
    <div class="node-endpoint-row mt-3">
      <OneuiInput
        v-model.trim="form.host"
        label="IP или домен"
        placeholder="relay.example.com"
        class="node-endpoint-host"
      />
      <OneuiInput v-model.number="form.port" label="gRPC порт" type="number" class="node-endpoint-port" />
    </div>
    <OneuiInput
      v-model.trim="form.token"
      label="Токен (bearer)"
      placeholder="оставьте пустым, чтобы не менять"
      class="mt-3"
    />
    <label class="field-label mt-4">WG-бэкенд провижна</label>
    <OneuiRadioGroup v-model="form.wg_backend" :options="wgBackendOptions" variant="pill" />
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
    </template>
    <p v-if="editError" class="state-error mt-2">{{ editError }}</p>
    <template #actions>
      <SamsungButton :busy="saving" @click="submitEdit">Сохранить</SamsungButton>
      <SamsungButton variant="secondary" :disabled="saving" @click="showEdit = false">Отмена</SamsungButton>
    </template>
  </SamsungModal>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { Pencil, Terminal, Trash2 } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungIconButton from '@/components/layout/SamsungIconButton.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungModal from '@/components/layout/SamsungModal.vue';
import SamsungPill from '@/components/layout/SamsungPill.vue';
import OneuiRadioGroup from '@/components/controls/OneuiRadioGroup.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import OneuiSelect from '@/components/controls/OneuiSelect.vue';
import TrafficChart from '@/components/domain/TrafficChart.vue';
import FlowGraph from '@/components/domain/FlowGraph.vue';
import WgPeers from '@/views/shared/WgPeers.vue';
import { connectAdminSocket } from '@/stores/admin-socket.js';
import { confirm } from '@/stores/confirm.js';
import { formatBytes, formatUnix } from '@/utils/format.js';

const props = defineProps({
  apiBase: { type: String, default: '/api/owner' },
  backName: { type: String, default: 'owner-nodes' },
});

const route = useRoute();
const router = useRouter();
const nodeId = route.params.id;

const traffic = ref(null);
const meta = ref(null);
const flows = ref([]);
const clientNames = ref({});
const xrayFlows = ref([]);
const xrayNames = ref({});
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

// Live per-node stats/flows over the WS (same as the node list), so the tiles and
// graph track ~1s instead of the slow REST refresh.
const liveByNode = ref({});
const flowsLive = ref({});
const liveTotals = computed(() => {
  const base = (traffic.value && traffic.value.totals) || {};
  const l = liveByNode.value[nodeId];
  if (!l || Date.now() - l.at > 10000) return base;
  return {
    ...base,
    cur_rx_rate: l.rx_rate || 0,
    cur_tx_rate: l.tx_rate || 0,
    active_sessions: l.active_sessions || 0,
    active_streams: l.active_streams || 0,
  };
});
const graphFlows = computed(() => {
  const l = flowsLive.value[nodeId];
  if (!l || Date.now() - l.at > 10000) return flows.value;
  return l.flows;
});
const statsReady = computed(() => !!traffic.value || !!liveByNode.value[nodeId]);

const backTo = computed(() => ({ name: props.backName }));
const node = computed(() => (traffic.value?.nodes || []).find((n) => n.id === nodeId) || null);
const isXui = computed(() => meta.value?.kind === 'xui');
const metaStatus = computed(() => meta.value?.status || node.value?.status || '');
const nodeKindLabel = computed(() => (meta.value?.kind === 'xui' ? '3x-ui' : meta.value?.kind ? 'VK TURN' : ''));
const curRate = computed(() => (liveTotals.value.cur_rx_rate || 0) + (liveTotals.value.cur_tx_rate || 0));
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

async function loadMeta() {
  try {
    const data = await fetchJSON(`${props.apiBase}/nodes`);
    meta.value = (data.nodes || []).find((n) => n.id === nodeId) || meta.value;
  } catch {
    // Non-fatal: the kind branch falls back to vk-turn until meta loads.
  }
}

async function load() {
  if (!meta.value) await loadMeta();
  try {
    const q = `node=${encodeURIComponent(nodeId)}`;
    if (isXui.value) {
      const x = await fetchJSON(`${props.apiBase}/stats/xrayflows?${q}`);
      xrayFlows.value = x.flows || [];
      xrayNames.value = x.client_names || {};
      loadError.value = '';
      return;
    }
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

// --- Header actions: connect command, edit wg-backend, delete ---
const showConnect = ref(false);
const connectCommand = ref('');
const connectError = ref('');
const connectCopied = ref(false);

// The relay must listen on exactly the port this node's endpoint names, so the
// command carries it rather than letting connect.sh fall back to its own default
// and leave the node unreachable for the collector.
function connectPortEnv(endpoint) {
  const port = String(endpoint || '').split(':').pop();
  return /^\d+$/.test(port) ? `VKTP_GRPC_PORT=${port} ` : '';
}

function buildConnectCommand(id, kind, token, endpoint) {
  const origin = window.location.origin;
  const grpc = `${window.location.hostname}:443`;
  const k = kind === 'xui' ? 'xui' : 'vktp';
  return `curl -fsSL ${origin}/connect.sh | ${connectPortEnv(endpoint)}sh -s -- grpc connect ${grpc} ${token || '<token>'} ${id} ${k}`;
}

async function openConnect() {
  showConnect.value = true;
  connectError.value = '';
  connectCommand.value = '';
  try {
    const data = await fetchJSON(`${props.apiBase}/nodes/${nodeId}/connect`);
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

async function copyConnect() {
  try {
    await navigator.clipboard.writeText(connectCommand.value);
    connectCopied.value = true;
    setTimeout(() => (connectCopied.value = false), 1500);
  } catch {
    // Clipboard denied; the command stays visible to copy manually.
  }
}

const showEdit = ref(false);
const saving = ref(false);
const editError = ref('');
const form = reactive({ name: '', host: '', port: 0, token: '', wg_backend: '', xui_node_id: '', xui_inbound_tag: '' });
const wgBackendOptions = [
  { value: '', label: 'Своя wg на ноде' },
  { value: 'own', label: 'Своя wg (явно)' },
  { value: 'xui', label: 'Клиент на 3x-ui инбаунде' },
];
const xuiNodes = ref([]);
const inboundOptions = ref([{ value: '', label: 'Не выбран' }]);
const inboundsLoading = ref(false);
const xuiNodeOptions = computed(() => [
  { value: '', label: 'Не выбран' },
  ...xuiNodes.value.map((n) => ({ value: n.id, label: n.name || n.id })),
]);

async function openEdit() {
  editError.value = '';
  const m = meta.value || {};
  form.name = m.name || '';
  const [host, port] = String(m.grpc_endpoint || '').split(':');
  form.host = host || '';
  form.port = Number(port) || 0;
  form.token = '';
  form.wg_backend = m.wg_backend || '';
  form.xui_node_id = m.xui_node_id || '';
  form.xui_inbound_tag = m.xui_inbound_tag || '';
  inboundOptions.value = [{ value: '', label: 'Не выбран' }];
  showEdit.value = true;
  try {
    const data = await fetchJSON(`${props.apiBase}/nodes`);
    xuiNodes.value = (data.nodes || []).filter((n) => n.kind === 'xui');
  } catch {
    // Non-fatal: xui-node select just stays empty.
  }
  if (form.wg_backend === 'xui' && form.xui_node_id) {
    const keepTag = form.xui_inbound_tag;
    await onXuiNodeChange(form.xui_node_id);
    form.xui_inbound_tag = keepTag;
  }
}

async function onXuiNodeChange(value) {
  form.xui_node_id = value;
  form.xui_inbound_tag = '';
  inboundOptions.value = [{ value: '', label: 'Не выбран' }];
  if (!value) return;
  inboundsLoading.value = true;
  try {
    const data = await fetchJSON(`${props.apiBase}/nodes/${value}/inbounds`);
    inboundOptions.value = [
      { value: '', label: 'Не выбран' },
      ...(data.inbounds || []).map((i) => ({ value: i.tag, label: `${i.remark || i.tag} (${i.protocol}:${i.port})` })),
    ];
  } catch {
    // Non-fatal: leave the placeholder.
  } finally {
    inboundsLoading.value = false;
  }
}

async function submitEdit() {
  saving.value = true;
  editError.value = '';
  try {
    const body = {
      name: form.name,
      kind: meta.value?.kind || 'vk_turn_proxy',
      grpc_endpoint: `${form.host}:${form.port}`,
      wg_backend: form.wg_backend,
      xui_node_id: form.wg_backend === 'xui' ? form.xui_node_id : '',
      xui_inbound_tag: form.wg_backend === 'xui' ? form.xui_inbound_tag : '',
    };
    if (form.token) body.grpc_token = form.token;
    const res = await fetch(`${props.apiBase}/nodes/${nodeId}`, {
      method: 'PUT',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(await res.text());
    showEdit.value = false;
    await loadMeta();
  } catch (err) {
    editError.value = err.message || 'Не удалось сохранить';
  } finally {
    saving.value = false;
  }
}

async function onDelete() {
  if (
    !(await confirm({
      title: 'Удалить ноду?',
      message: `Нода «${meta.value?.name || nodeId}» будет удалена из панели.`,
      confirmText: 'Удалить',
      danger: true,
    }))
  ) {
    return;
  }
  try {
    const res = await fetch(`${props.apiBase}/nodes/${nodeId}`, { method: 'DELETE', credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    router.push({ name: props.backName });
  } catch (err) {
    loadError.value = err.message || 'Не удалось удалить';
  }
}

onMounted(() => {
  load();
  // Slow REST refresh for DB-backed parts; live tiles + flow graph come over the WS.
  timer = setInterval(load, 20000);
  socketHandle = connectAdminSocket((event) => {
    if (event.kind === 'node_stats' && event.payload && event.payload.node_id === nodeId) {
      liveByNode.value = { ...liveByNode.value, [nodeId]: { ...event.payload, at: Date.now() } };
    } else if (event.kind === 'node_flows' && event.payload && event.payload.node_id === nodeId) {
      flowsLive.value = { ...flowsLive.value, [nodeId]: { flows: event.payload.flows || [], at: Date.now() } };
    }
  });
});
onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
  if (socketHandle) socketHandle.close();
});
</script>

<style scoped>
.node-detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}
.node-detail-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
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
.connect-cmd {
  white-space: pre-wrap;
  word-break: break-all;
  background: rgba(255, 255, 255, 0.05);
  padding: 12px;
  border-radius: 10px;
  font-size: 12px;
}
</style>
