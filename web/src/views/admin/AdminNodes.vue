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

    <form v-if="allowGRPC" class="mt-4" @submit.prevent="onAdd">
      <OneuiRadioGroup v-model="form.kind" :options="kindOptions" variant="pill" />
      <div class="form-grid mt-3">
        <OneuiInput v-model.trim="form.name" label="Название" placeholder="Мой релей" />
        <OneuiInput v-model.trim="form.host" label="IP или домен" placeholder="relay.example.com" />
        <OneuiInput v-model.number="form.port" label="gRPC порт" type="number" :placeholder="String(defaultPort)" />
        <OneuiInput v-model.trim="form.grpc_token" label="Токен" placeholder="bearer-токен (опционально)" />
      </div>
      <p v-if="addError" class="state-error mt-2">{{ addError }}</p>
      <div class="actions-row mt-3">
        <SamsungButton type="submit" :busy="adding">
          <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
          Добавить сервер
        </SamsungButton>
      </div>
    </form>
  </section>

  <SamsungCard v-if="nodes.length" class="mt-6" title="Ноды" subtitle="Ваши подключённые серверы.">
    <ul class="admin-list mt-4">
      <li v-for="n in nodes" :key="n.id" class="session-row">
        <div>
          <strong class="session-row-actor">{{ n.name || n.id }}</strong>
          <SamsungPill :variant="n.status === 'online' ? 'online' : 'offline'" class="ml-2">
            {{ n.status }}
          </SamsungPill>
          <span class="admin-muted ml-2">{{ n.grpc_endpoint }}</span>
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
      <TrafficSparkline :series="traffic.series || []" class="mt-4" />
    </SamsungCard>

    <SamsungCard class="mt-6" title="Цепочки соединений" subtitle="Клиент → поток → назначение.">
      <FlowChain :flows="flows" class="mt-4" />
    </SamsungCard>

    <SamsungCard class="mt-6" title="Журнал соединений" subtitle="Недавние соединения через ваши ноды.">
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
    </SamsungCard>
  </template>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { Plus, Trash2 } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungIconButton from '@/components/layout/SamsungIconButton.vue';
import SamsungModal from '@/components/layout/SamsungModal.vue';
import SamsungPill from '@/components/layout/SamsungPill.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import OneuiRadioGroup from '@/components/controls/OneuiRadioGroup.vue';
import OneuiTextarea from '@/components/controls/OneuiTextarea.vue';
import TrafficSparkline from '@/components/domain/TrafficSparkline.vue';
import FlowChain from '@/components/domain/FlowChain.vue';
import { connectAdminSocket } from '@/stores/admin-socket.js';
import { formatBytes, formatUnix } from '@/utils/format.js';

const nodes = ref([]);
const allowGRPC = ref(false);
const traffic = ref(null);
const flows = ref([]);
const connections = ref([]);
const loadError = ref('');
const addError = ref('');
const adding = ref(false);

const clientNode = ref(null);
const clientPayload = ref('');
const clientAdding = ref(false);
const clientError = ref('');

const kindOptions = [
  { value: 'vk_turn_proxy', label: 'VK TURN' },
  { value: 'xui', label: '3x-ui' },
];
// Default gRPC ports: VK TURN relay 25612, 3x-ui panel API 25613.
const defaultPorts = { vk_turn_proxy: 25612, xui: 25613 };
const form = reactive({ name: '', kind: 'vk_turn_proxy', host: '', port: 25612, grpc_token: '' });
const defaultPort = computed(() => defaultPorts[form.kind] || 25612);
watch(
  () => form.kind,
  (kind) => {
    form.port = defaultPorts[kind] || form.port;
  },
);

const formatTs = formatUnix;

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
    return;
  }
  try {
    const [t, f, c] = await Promise.all([
      fetchJSON('/api/admin/stats/traffic'),
      fetchJSON('/api/admin/stats/flows'),
      fetchJSON('/api/admin/stats/connections?limit=50'),
    ]);
    traffic.value = t;
    flows.value = f.flows || [];
    connections.value = c.connections || [];
  } catch (err) {
    loadError.value = err.message || 'Не удалось загрузить статистику';
  }
}

async function refresh() {
  await loadNodes();
  await loadStats();
}

async function onAdd() {
  addError.value = '';
  const host = form.host.trim();
  if (!host) {
    addError.value = 'Укажите IP или домен';
    return;
  }
  const port = Number(form.port) || defaultPort.value;
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
    form.name = '';
    form.host = '';
    form.grpc_token = '';
    await refresh();
  } catch (err) {
    addError.value = err.message || 'Не удалось добавить';
  } finally {
    adding.value = false;
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
  timer = setInterval(loadStats, 5000);
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
.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 14px;
}
.node-row-tail {
  display: flex;
  align-items: center;
  gap: 12px;
}
</style>
