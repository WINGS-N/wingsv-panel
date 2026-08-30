<template>
  <!-- Rendered only when the operator turned the federation on. Off means the
       section does not exist, not that it exists and is greyed out. -->
  <section v-if="enabled" class="surface-card">
    <h2 class="section-title">Федерация</h2>
    <p class="body-copy">
      Отдайте мощности своего сервера в общий пул — с него будут обслуживаться бесплатные пользователи в пределах
      месячного лимита, который вы назначаете сами.
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>

    <div class="admin-stats">
      <div class="stat">
        <span class="stat-kicker">Ваших узлов</span>
        <span class="stat-value">{{ summary.nodes_online }} / {{ summary.nodes }}</span>
        <span class="stat-meta">онлайн сейчас</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Сессий</span>
        <span class="stat-value">{{ summary.sessions }}</span>
        <span class="stat-meta">активных подключений</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Отдача</span>
        <span class="stat-value"><span class="traffic-tx">↑</span> {{ rate(summary.up_rate_bps) }}</span>
        <span class="stat-meta">с ваших узлов</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Приём</span>
        <span class="stat-value"><span class="traffic-rx">↓</span> {{ rate(summary.down_rate_bps) }}</span>
        <span class="stat-meta">на устройства</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Отдано за период</span>
        <span class="stat-value">{{ bytes(summary.used_bytes) }}</span>
        <span class="stat-meta">из {{ bytes(summary.declared_budget_bytes) }}</span>
      </div>
    </div>

    <div class="actions-row mt-4">
      <SamsungButton :busy="minting" @click="mint">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        Подключить сервер
      </SamsungButton>
      <label class="federation-uses">
        <span class="admin-muted">серверов на один токен</span>
        <input v-model.number="uses" type="number" min="1" max="64" class="federation-uses-input" />
      </label>
      <SamsungButton variant="ghost" :busy="loading" @click="load">Обновить</SamsungButton>
    </div>

    <div v-if="command" class="entry-card mt-4">
      <p class="body-copy">
        Выполните на сервере, который отдаёте.
        <template v-if="mintedUses > 1"> Токен рассчитан на {{ mintedUses }} серверов и скоро протухнет. </template>
        <template v-else> Токен одноразовый и скоро протухнет. </template>
      </p>
      <CopyableLink :value="command" class="mt-3" />
    </div>
  </section>

  <section v-if="enabled && summary.node_list.length" class="surface-card mt-6">
    <h2 class="section-title">Узлы</h2>
    <!-- Строки без рамок, только разделители - как список профилей в DeX -->
    <div class="fed-node-list">
      <div v-for="node in summary.node_list" :key="node.id" class="fed-node-row">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="truncate text-[17px]">{{ node.hostname || node.id.slice(0, 8) }}</span>
            <span class="admin-pill" :class="node.state === 'active' ? 'is-online' : 'is-offline'">
              {{ stateLabel(node.state) }}
            </span>
          </div>
          <span v-if="node.reason" class="mt-0.5 block truncate text-sm text-wings-muted">{{ node.reason }}</span>
          <span class="mt-1 flex flex-wrap items-center gap-4 text-[13px] text-wings-muted">
            <span class="inline-flex items-center gap-1">
              <Users :size="13" aria-hidden="true" />{{ node.sessions }}
            </span>
            <span class="inline-flex items-center gap-1">
              <ArrowUp :size="13" :style="{ color: FLOW_UP }" aria-hidden="true" />{{ bytes(node.used_bytes) }}
              <span class="text-wings-kicker">из {{ bytes(node.declared_budget_bytes) }}</span>
            </span>
          </span>
          <!-- Полоса бюджета: цифры выше говорят сколько, она - насколько близко
               узел к тому, чтобы выйти из ротации -->
          <span class="fed-node-track" aria-hidden="true">
            <span class="fed-node-fill" :style="{ width: budgetPct(node) + '%' }"></span>
          </span>
        </div>

        <SamsungButton
          variant="ghost"
          :busy="busyNode === node.id"
          @click="setState(node, node.state === 'parked' ? 'active' : 'parked')"
        >
          <template #icon>
            <PlayCircle v-if="node.state === 'parked'" class="button-icon" aria-hidden="true" />
            <PauseCircle v-else class="button-icon" aria-hidden="true" />
          </template>
          {{ node.state === 'parked' ? 'Вернуть' : 'Снять' }}
        </SamsungButton>
      </div>
    </div>
  </section>
</template>

<script setup>
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { ArrowUp, PauseCircle, PlayCircle, Plus, Users } from 'lucide-vue-next';

// Тот же цветовой код направления, что в приложении
const FLOW_UP = '#0381fe';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import CopyableLink from '@/components/domain/CopyableLink.vue';
import { formatBytes } from '@/utils/format';

const enabled = ref(false);
const loading = ref(false);
const minting = ref(false);
// Сколько серверов вступит по одному токену. Больше одного нужно там, где
// ноды поднимаются из общего секрета - в кубере это DaemonSet.
const uses = ref(1);
const mintedUses = ref(1);
const busyNode = ref('');
const loadError = ref('');
const command = ref('');
const summary = reactive({
  nodes: 0,
  nodes_online: 0,
  sessions: 0,
  up_rate_bps: 0,
  down_rate_bps: 0,
  used_bytes: 0,
  declared_budget_bytes: 0,
  node_list: [],
});

let timer = null;

onMounted(() => {
  load();
  // The head recomputes rotation every ten seconds, so anything faster only adds
  // load without telling the donor anything new.
  timer = setInterval(load, 15000);
});

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});

async function load() {
  loading.value = true;
  try {
    const res = await fetch('/api/admin/federation/summary', { credentials: 'include' });
    if (!res.ok) throw new Error(await errorText(res));
    const data = await res.json();
    enabled.value = Boolean(data.enabled);
    Object.assign(summary, { ...data, node_list: data.node_list || [] });
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    loading.value = false;
  }
}

async function mint() {
  minting.value = true;
  try {
    const res = await fetch('/api/admin/federation/enroll-token', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ uses: Math.max(1, Number(uses.value) || 1) }),
    });
    if (!res.ok) throw new Error(await errorText(res));
    const got = await res.json();
    command.value = got.command;
    mintedUses.value = got.uses || 1;
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    minting.value = false;
  }
}

async function setState(node, state) {
  busyNode.value = node.id;
  try {
    const res = await fetch(`/api/admin/federation/nodes/${node.id}/state`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ state, reason: state === 'parked' ? 'снято владельцем' : '' }),
    });
    if (!res.ok) throw new Error(await errorText(res));
    await load();
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    busyNode.value = '';
  }
}

async function errorText(res) {
  try {
    return (await res.json()).error || `HTTP ${res.status}`;
  } catch {
    return `HTTP ${res.status}`;
  }
}

function bytes(value) {
  return formatBytes(value || 0);
}

function rate(bytesPerSecond) {
  const bits = (bytesPerSecond || 0) * 8;
  const units = ['бит/с', 'Кбит/с', 'Мбит/с', 'Гбит/с'];
  let index = 0;
  let value = bits;
  while (value >= 1000 && index < units.length - 1) {
    value /= 1000;
    index += 1;
  }
  return `${value >= 100 || index === 0 ? Math.round(value) : value.toFixed(1)} ${units[index]}`;
}

// Доля выбранного бюджета. Именно она решает судьбу узла: около 85 процентов он
// перестаёт получать новых пользователей, около 97 снимается из ротации
function budgetPct(node) {
  const limit = Number(node.declared_budget_bytes) || 0;
  if (limit <= 0) return 0;
  return Math.min(100, Math.round((Number(node.used_bytes) / limit) * 100));
}

function stateLabel(state) {
  switch (state) {
    case 'active':
      return 'в ротации';
    case 'draining':
      return 'выводится';
    case 'parked':
      return 'снят';
    case 'quarantined':
      return 'карантин';
    default:
      return state || 'неизвестно';
  }
}
</script>
