<template>
  <!-- Rendered only when the operator turned the federation on. Off means the
       section does not exist, not that it exists and is greyed out. -->
  <section v-if="enabled" class="surface-card">
    <div class="federation-live-head">
      <h2 class="section-title">Федерация</h2>
      <!-- Просто точка: она говорит "цифры живые" одним своим видом, подпись
           к ней только шумела бы -->
      <span class="live-dot" role="status" aria-label="Данные обновляются" title="Данные обновляются"></span>
    </div>
    <p class="body-copy body-copy-wide">
      Отдайте мощности своего сервера в общий пул — с него будут обслуживаться бесплатные пользователи в пределах
      месячного лимита, который вы назначаете сами.
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>

    <div class="admin-stats">
      <div class="stat">
        <span class="stat-kicker">
          <Server :size="14" class="stat-kicker-icon" aria-hidden="true" />
          Ваших нод
        </span>
        <span class="stat-value">{{ summary.nodes_online }} / {{ summary.nodes }}</span>
        <span class="stat-meta">онлайн сейчас</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <Users :size="14" class="stat-kicker-icon" aria-hidden="true" />
          Сессий
        </span>
        <span class="stat-value">{{ summary.sessions }}</span>
        <span class="stat-meta">активных подключений</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <ArrowUp :size="14" class="stat-kicker-icon" :style="{ color: FLOW_UP }" aria-hidden="true" />
          Отдача
        </span>
        <span class="stat-value"><span class="traffic-tx">↑</span> {{ rate(summary.up_rate_bps) }}</span>
        <span class="stat-meta">с ваших нод</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <ArrowDown :size="14" class="stat-kicker-icon" :style="{ color: FLOW_DOWN }" aria-hidden="true" />
          Приём
        </span>
        <span class="stat-value"><span class="traffic-rx">↓</span> {{ rate(summary.down_rate_bps) }}</span>
        <span class="stat-meta">на устройства</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <CalendarRange :size="14" class="stat-kicker-icon" aria-hidden="true" />
          Отдано за период
        </span>
        <span class="stat-value">{{ bytes(summary.used_bytes) }}</span>
        <span class="stat-meta">из {{ bytes(summary.declared_budget_bytes) }}</span>
      </div>
    </div>

    <div class="form-grid mt-5">
      <OneuiInput v-model.number="uses" label="Серверов на один токен" type="number" :min="1" :max="64" />
    </div>

    <div class="actions-row">
      <SamsungButton :busy="minting" @click="mint">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        Подключить сервер
      </SamsungButton>
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
    <h2 class="section-title">Ноды</h2>
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
               нода к тому, чтобы выйти из ротации -->
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
import { ArrowDown, ArrowUp, CalendarRange, PauseCircle, PlayCircle, Plus, Server, Users } from 'lucide-vue-next';
import { connectAdminSocket } from '@/stores/admin-socket.js';

// Тот же цветовой код направления, что в приложении
const FLOW_UP = '#0381fe';
const FLOW_DOWN = '#15b76f';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
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
let socketHandle = null;

onMounted(() => {
  load();
  // Список нод и их состояния приходят по REST: они меняются медленно, и
  // опрашивать их чаще смысла нет
  timer = setInterval(load, 20000);
  // А цифры сверху идут потоком, поэтому кнопки "обновить" на этом экране нет:
  // человеку не должно приходиться думать о свежести данных
  socketHandle = connectAdminSocket((event) => {
    if (event.kind !== 'fed_global' || !event.payload) return;
    const live = event.payload;
    summary.nodes_online = live.nodes_online ?? summary.nodes_online;
    summary.sessions = live.users_online ?? summary.sessions;
    summary.up_rate_bps = live.up_rate_bps ?? summary.up_rate_bps;
    summary.down_rate_bps = live.down_rate_bps ?? summary.down_rate_bps;
  });
});

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
  if (socketHandle) {
    socketHandle.close();
    socketHandle = null;
  }
});

async function load() {
  loading.value = true;
  try {
    const res = await fetch('/api/admin/federation/summary', { credentials: 'include' });
    if (!res.ok) throw new Error(await errorText(res));
    const data = await res.json();
    enabled.value = Boolean(data.enabled);
    // Голова могла отвалиться: раздел остаётся, цифры замирают, причина видна
    loadError.value = data.error || '';
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
  const units = ['bit/s', 'Kbit/s', 'Mbit/s', 'Gbit/s'];
  let index = 0;
  let value = bits;
  while (value >= 1000 && index < units.length - 1) {
    value /= 1000;
    index += 1;
  }
  return `${value >= 100 || index === 0 ? Math.round(value) : value.toFixed(1)} ${units[index]}`;
}

// Доля выбранного бюджета. Именно она решает судьбу ноды: около 85 процентов он
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
