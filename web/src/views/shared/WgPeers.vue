<template>
  <SamsungCard
    class="mt-6"
    :title="`WG-конфиги (${peers.length})`"
    subtitle="Выданные клиентам WireGuard-пиры на нодах. Отзыв удаляет запись в панели."
  >
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <div class="wg-filter mt-2">
      <OneuiInput v-model.trim="query" label="Поиск" placeholder="клиент, нода, адрес или ключ" />
    </div>
    <ul class="admin-list mt-4">
      <li v-for="p in filtered" :key="p.client_id + p.node_id" class="session-row">
        <div class="wg-peer-main">
          <strong class="session-row-actor">{{ p.client_name || p.client_id }}</strong>
          <span class="admin-muted ml-2">на {{ p.node_name || p.node_id }}</span>
          <div class="wg-peer-meta">
            <span class="wg-tag">{{ p.allowed_ips || '—' }}</span>
            <code class="admin-mono wg-key" :title="p.public_key">{{ shortKey(p.public_key) }}</code>
            <span class="admin-muted">{{ formatTs(p.created_at) }}</span>
          </div>
        </div>
        <SamsungButton variant="danger" size="small" :busy="revoking === p.client_id + p.node_id" @click="revoke(p)">
          Отозвать
        </SamsungButton>
      </li>
      <li v-if="!filtered.length" class="session-row">
        <span class="admin-muted">{{ peers.length ? 'Ничего не найдено.' : 'Выданных пиров пока нет.' }}</span>
      </li>
    </ul>
  </SamsungCard>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import { formatUnix } from '@/utils/format.js';
import { confirm } from '@/stores/confirm.js';

const props = defineProps({
  apiBase: { type: String, default: '/api/admin' },
  // When set, only this node's peers are shown (node detail view).
  nodeId: { type: String, default: '' },
});

const peers = ref([]);
const loadError = ref('');
const query = ref('');
const revoking = ref('');
const formatTs = formatUnix;

const filtered = computed(() => {
  const q = query.value.toLowerCase();
  const scoped = props.nodeId ? peers.value.filter((p) => p.node_id === props.nodeId) : peers.value;
  if (!q) return scoped;
  return scoped.filter((p) =>
    [p.client_name, p.client_id, p.node_name, p.node_id, p.allowed_ips, p.public_key]
      .filter(Boolean)
      .some((v) => v.toLowerCase().includes(q)),
  );
});

function shortKey(key) {
  if (!key) return '—';
  return key.length > 16 ? `${key.slice(0, 8)}...${key.slice(-6)}` : key;
}

async function load() {
  try {
    const res = await fetch(`${props.apiBase}/wgpeers`, { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const body = await res.json();
    peers.value = body.peers || [];
    loadError.value = '';
  } catch (err) {
    loadError.value = err.message || 'Не удалось загрузить пиры';
  }
}

async function revoke(p) {
  if (
    !(await confirm({
      title: 'Отозвать конфиг',
      message: `Отозвать конфиг клиента ${p.client_name || p.client_id} на ${p.node_name || p.node_id}? Пир будет удалён и с ноды.`,
      confirmText: 'Отозвать',
      danger: true,
    }))
  ) {
    return;
  }
  revoking.value = p.client_id + p.node_id;
  try {
    const res = await fetch(
      `${props.apiBase}/wgpeers/${encodeURIComponent(p.client_id)}/${encodeURIComponent(p.node_id)}`,
      { method: 'DELETE', credentials: 'include' },
    );
    if (!res.ok) throw new Error(await res.text());
    await load();
  } catch (err) {
    loadError.value = err.message || 'Не удалось отозвать';
  } finally {
    revoking.value = '';
  }
}

onMounted(load);
</script>

<style scoped>
.wg-filter {
  max-width: 360px;
}
.wg-peer-main {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.wg-peer-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  font-size: 12px;
}
.wg-tag {
  background: rgba(75, 141, 255, 0.16);
  border-radius: 6px;
  padding: 1px 8px;
  font-size: 12px;
}
.wg-key {
  font-size: 12px;
  color: rgba(252, 252, 252, 0.6);
}
</style>
