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
          Downlink
        </span>
        <span class="stat-value"><span class="traffic-tx">↑</span> {{ rate(summary.down_rate_bps) }}</span>
        <span class="stat-meta">с ваших нод</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <ArrowDown :size="14" class="stat-kicker-icon" :style="{ color: FLOW_DOWN }" aria-hidden="true" />
          Uplink
        </span>
        <span class="stat-value"><span class="traffic-rx">↓</span> {{ rate(summary.up_rate_bps) }}</span>
        <span class="stat-meta">на ваши ноды</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <CalendarRange :size="14" class="stat-kicker-icon" aria-hidden="true" />
          Отдано за период
        </span>
        <span class="stat-value">{{ bytes(summary.used_bytes) }}</span>
        <span class="stat-meta">
          из {{ bytes(summary.declared_budget_bytes)
          }}<template v-if="summary.probe_bytes">
            &middot; из них проверочного трафика {{ bytes(summary.probe_bytes) }}</template
          >
        </span>
      </div>
    </div>

    <div class="form-grid mt-5">
      <OneuiInput v-model.number="uses" label="Серверов на один токен" type="number" :min="1" :max="64" />
      <OneuiInput
        v-model.number="newNodeBudgetGb"
        label="Отдаю в месяц, GB"
        type="number"
        :min="0"
        :max="1048576"
        placeholder="по умолчанию площадки"
      />
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

  <SamsungModal :model-value="Boolean(removing)" title="Убрать сервер?" @update:model-value="removing = null">
    <p class="body-copy">
      Сервер <b>{{ removing?.hostname || removing?.id?.slice(0, 12) }}</b> перестанет выдаваться людям, а те, кто на нём
      сидит, переедут на другие. Ваш агент можно будет просто остановить.
    </p>
    <p class="admin-muted mt-2">Вернуть сервер потом можно, зачислив его заново новым токеном.</p>
    <p v-if="removeError" class="state-error mt-3">{{ removeError }}</p>
    <template #actions>
      <SamsungButton :busy="busyNode === removing?.id" @click="confirmRemove">
        <template #icon><Trash2 class="button-icon" aria-hidden="true" /></template>
        Убрать
      </SamsungButton>
      <SamsungButton variant="secondary" @click="removing = null">Отмена</SamsungButton>
    </template>
  </SamsungModal>

  <section v-if="enabled && payouts.enabled" class="surface-card mt-6">
    <div class="federation-live-head">
      <h2 class="section-title">Выплаты</h2>
      <span v-if="payouts.total_micro" class="admin-pill is-info">{{ usdt(payouts.total_micro) }} USDT начислено</span>
    </div>
    <p class="admin-muted mt-1">Начисляется за трафик, который подписали клиенты. Период закрывается раз в неделю.</p>
    <p v-if="payouts.note" class="state-hint mt-3">{{ payouts.note }}</p>
    <template v-else>
      <div class="mt-4 flex flex-wrap items-end gap-3">
        <OneuiInput
          v-model="walletDraft"
          label="Кошелёк Solana (USDT)"
          placeholder="адрес, на который платить"
          class="min-w-[280px] flex-1"
        />
        <SamsungButton :busy="savingWallet" @click="saveWallet">
          <template #icon><Wallet class="button-icon" aria-hidden="true" /></template>
          Сохранить
        </SamsungButton>
      </div>
      <p v-if="walletError" class="state-error mt-2">{{ walletError }}</p>
      <p v-else-if="!payouts.address" class="state-hint mt-2">
        Без кошелька начисления копятся, но в расчётный период не попадают.
      </p>

      <div v-if="payouts.epochs.length" class="fed-node-list mt-4">
        <div v-for="epoch in payouts.epochs" :key="epoch.number" class="fed-node-row">
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-[15px]">Эпоха {{ epoch.number }}</span>
              <span class="admin-pill">{{ usdt(epoch.amount_micro) }} USDT</span>
              <span class="admin-pill" :class="epoch.tx_ref ? 'is-online' : 'is-info'">
                {{ epoch.tx_ref ? 'можно забирать' : 'посчитана' }}
              </span>
            </div>
            <span class="mt-1 flex flex-wrap items-center gap-3 text-[13px] text-wings-muted">
              <span>{{ epochDate(epoch.start_unix) }} - {{ epochDate(epoch.end_unix) }}</span>
            </span>
          </div>
        </div>
      </div>
      <p v-else class="state-hint mt-4">Ни один период ещё не закрыт.</p>
    </template>
  </section>

  <section v-if="enabled && summary.months.length" class="surface-card mt-6">
    <h2 class="section-title">По месяцам</h2>
    <p class="admin-muted mt-1">Сколько трафика ушло через ваши серверы. Счётчик сверху обнуляется, этот - нет.</p>
    <div class="fed-months mt-4">
      <div v-for="m in summary.months" :key="m.month" class="fed-month">
        <span class="fed-month-name">{{ monthName(m.month) }}</span>
        <span class="fed-month-track" aria-hidden="true">
          <span class="fed-month-fill" :style="{ width: monthPct(m) + '%' }"></span>
        </span>
        <span class="fed-month-value">{{ bytes(m.bytes) }}</span>
      </div>
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
            <!-- Точка перед подписью: состояние читается раньше, чем текст -->
            <span class="admin-pill" :class="stateClass(node.state)">
              <span class="state-dot" :class="dotClass(node.state)" aria-hidden="true"></span>
              {{ stateLabel(node.state) }}
            </span>
          </div>
          <span v-if="node.reason" class="mt-0.5 block truncate text-sm text-wings-muted">{{ node.reason }}</span>
          <span class="mt-1 flex flex-wrap items-center gap-4 text-[13px] text-wings-muted">
            <span class="inline-flex items-center gap-1">
              <Users :size="13" aria-hidden="true" />{{ node.sessions }}
            </span>
            <span v-if="node.xray_version" class="inline-flex items-center gap-1" title="Сборка Xray на ноде">
              <Boxes :size="13" aria-hidden="true" />{{ node.xray_version }}
            </span>
            <span v-if="node.vktp_version" class="inline-flex items-center gap-1" title="Сборка релея на ноде">
              <Radio :size="13" aria-hidden="true" />{{ node.vktp_version }}
            </span>
            <span class="inline-flex items-center gap-1">
              <ArrowUp :size="13" :style="{ color: FLOW_UP }" aria-hidden="true" />{{ bytes(node.used_bytes) }}
              <span class="text-wings-kicker">из {{ bytes(node.declared_budget_bytes) }}</span>
              <span
                v-if="node.probe_bytes"
                class="text-wings-kicker"
                title="Трафик наших зондов, которым мы проверяем ноду. В ваш лимит не входит"
              >
                &middot; проверки {{ bytes(node.probe_bytes) }}
              </span>
              <button type="button" class="fed-node-edit" title="Изменить месячный лимит" @click="startBudget(node)">
                <Pencil :size="13" aria-hidden="true" />
              </button>
            </span>
            <span v-if="budgetFor === node.id" class="mt-2 flex flex-wrap items-center gap-2">
              <input v-model.number="budgetGb" class="fed-budget-input" type="number" min="1" step="1" />
              <span class="text-wings-kicker">GB в месяц</span>
              <SamsungButton :busy="busyNode === node.id" @click="saveBudget(node)">Сохранить</SamsungButton>
              <SamsungButton variant="ghost" @click="budgetFor = ''">Отмена</SamsungButton>
            </span>
          </span>
          <!-- Полоса показывает ОСТАТОК: полная означает, что лимит цел, и
               тает по мере того, как его съедают -->
          <span class="fed-node-track" aria-hidden="true">
            <span class="fed-node-fill" :class="budgetClass(node)" :style="{ width: budgetLeftPct(node) + '%' }"></span>
          </span>
        </div>

        <SamsungButton variant="ghost" :busy="busyNode === node.id" @click="askRemove(node)">
          <template #icon><Trash2 class="button-icon" aria-hidden="true" /></template>
          Убрать
        </SamsungButton>
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
import {
  ArrowDown,
  ArrowUp,
  Boxes,
  Radio,
  CalendarRange,
  PauseCircle,
  Pencil,
  PlayCircle,
  Plus,
  Server,
  Trash2,
  Users,
  Wallet,
} from 'lucide-vue-next';

// Тот же цветовой код направления, что в приложении
const FLOW_UP = '#0381fe';
const FLOW_DOWN = '#15b76f';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungModal from '@/components/layout/SamsungModal.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import CopyableLink from '@/components/domain/CopyableLink.vue';
import { formatBytes, formatSpeed as rate, formatUsdt as usdt } from '@/utils/format';

const enabled = ref(false);
const loading = ref(false);
const minting = ref(false);
// Сколько серверов вступит по одному токену. Больше одного нужно там, где
// ноды поднимаются из общего секрета - в кубере это DaemonSet.
const uses = ref(1);
// Потолок для новой машины. Ноль оставляет общий потолок площадки: донор не
// всегда хочет его трогать
const newNodeBudgetGb = ref(0);
// Ноль оставляет общий потолок площадки: донор не всегда хочет его трогать
const mintedUses = ref(1);
const busyNode = ref('');
const removing = ref(null);
const removeError = ref('');
const budgetFor = ref('');
const budgetGb = ref(0);
const loadError = ref('');
const command = ref('');
const summary = reactive({
  nodes: 0,
  nodes_online: 0,
  sessions: 0,
  up_rate_bps: 0,
  down_rate_bps: 0,
  used_bytes: 0,
  probe_bytes: 0,
  declared_budget_bytes: 0,
  node_list: [],
  months: [],
});

// Выплаты живут своей жизнью: их считают раз в период, и дёргать их вместе с
// живыми счётчиками незачем
const payouts = reactive({ enabled: false, note: '', address: '', total_micro: 0, claimable_micro: 0, epochs: [] });
const walletDraft = ref('');
const walletError = ref('');
const savingWallet = ref(false);

let timer = null;
let live = null;

onMounted(() => {
  load();
  loadPayouts();
  // Список нод и их состояния приходят по REST: они меняются медленно, и
  // опрашивать их чаще смысла нет
  timer = setInterval(load, 20000);
  // А цифры сверху идут потоком, поэтому кнопки "обновить" на этом экране нет:
  // человеку не должно приходиться думать о свежести данных
  live = new EventSource('/api/admin/federation/live', { withCredentials: true });
  live.onmessage = (event) => {
    try {
      Object.assign(summary, JSON.parse(event.data));
    } catch {
      // Полуприехавший кадр просто пропускаем: следующий придёт через секунду
    }
  };
});

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
  if (live) {
    live.close();
    live = null;
  }
});

function askRemove(node) {
  removing.value = node;
  removeError.value = '';
}

async function confirmRemove() {
  const node = removing.value;
  if (!node) return;
  busyNode.value = node.id;
  removeError.value = '';
  try {
    const res = await fetch(`/api/admin/federation/nodes/${encodeURIComponent(node.id)}`, {
      method: 'DELETE',
      credentials: 'include',
    });
    if (!res.ok) throw new Error(await errorText(res));
    removing.value = null;
    await load();
  } catch (err) {
    removeError.value = String(err.message || err);
  } finally {
    busyNode.value = '';
  }
}

async function loadPayouts() {
  try {
    const res = await fetch('/api/admin/federation/payouts', { credentials: 'include' });
    if (!res.ok) throw new Error(await errorText(res));
    const data = await res.json();
    Object.assign(payouts, {
      enabled: Boolean(data.enabled),
      note: data.error || '',
      address: data.address || '',
      total_micro: Number(data.total_micro || 0),
      claimable_micro: Number(data.claimable_micro || 0),
      epochs: data.epochs || [],
    });
    // Поле не перетираем, пока человек в нём печатает
    if (!walletDraft.value) walletDraft.value = payouts.address;
  } catch {
    // Выплаты - не повод завалить весь раздел: счётчики трафика важнее
  }
}

async function saveWallet() {
  savingWallet.value = true;
  walletError.value = '';
  try {
    const res = await fetch('/api/admin/federation/payouts/address', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ address: walletDraft.value.trim() }),
    });
    if (!res.ok) throw new Error(await errorText(res));
    await loadPayouts();
  } catch (err) {
    walletError.value = String(err.message || err);
  } finally {
    savingWallet.value = false;
  }
}

function epochDate(unix) {
  if (!unix) return '';
  return new Date(Number(unix) * 1000).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short' });
}

async function load() {
  loading.value = true;
  try {
    const res = await fetch('/api/admin/federation/summary', { credentials: 'include' });
    if (!res.ok) throw new Error(await errorText(res));
    const data = await res.json();
    enabled.value = Boolean(data.enabled);
    // Башка могла отвалиться: раздел остаётся, цифры замирают, причина видна
    loadError.value = data.error || '';
    Object.assign(summary, { ...data, node_list: data.node_list || [], months: data.months || [] });
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
      body: JSON.stringify({
        uses: Math.max(1, Number(uses.value) || 1),
        budget_gb: Math.max(0, Number(newNodeBudgetGb.value) || 0),
      }),
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

// Донор правит лимит в GB: байты он всё равно набирает из башки, а руками
// вводить их некому
function startBudget(node) {
  budgetFor.value = node.id;
  budgetGb.value = Math.max(1, Math.round(Number(node.declared_budget_bytes || 0) / 1024 ** 3));
}

async function saveBudget(node) {
  const gb = Math.max(1, Math.round(Number(budgetGb.value) || 0));
  busyNode.value = node.id;
  try {
    const res = await fetch(`/api/admin/federation/nodes/${node.id}/budget`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ declared_budget_bytes: gb * 1024 ** 3 }),
    });
    if (!res.ok) throw new Error(await errorText(res));
    node.declared_budget_bytes = gb * 1024 ** 3;
    budgetFor.value = '';
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    busyNode.value = '';
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

// Доля выбранного бюджета. Именно она решает судьбу ноды: около 85 процентов он
// перестаёт получать новых пользователей, около 97 снимается с выдачи
// Полоса меряется относительно лучшего месяца: абсолютной шкалы у пожертвований
// нет, а сравнивать с прошлым месяцем человек и так будет
function monthPct(entry) {
  const top = Math.max(...summary.months.map((m) => Number(m.bytes) || 0), 1);
  return Math.max(2, Math.round((Number(entry.bytes) / top) * 100));
}

const MONTHS = [
  'январь',
  'февраль',
  'март',
  'апрель',
  'май',
  'июнь',
  'июль',
  'август',
  'сентябрь',
  'октябрь',
  'ноябрь',
  'декабрь',
];

function monthName(value) {
  const [year, month] = String(value).split('-');
  const name = MONTHS[Number(month) - 1];
  if (!name) return value;
  return Number(year) === new Date().getFullYear() ? name : `${name} ${year}`;
}

// Остаток лимита в процентах. Полная полоса - лимит нетронут
function budgetLeftPct(node) {
  const limit = Number(node.declared_budget_bytes) || 0;
  if (limit <= 0) return 100;
  const left = limit - Number(node.used_bytes);
  return Math.max(0, Math.min(100, Math.round((left / limit) * 100)));
}

// Цвет предупреждает раньше, чем цифра: зелёный - запас есть, жёлтый - лимит на
// исходе, красный - нода вот-вот выйдет из выдачи
function budgetClass(node) {
  const left = budgetLeftPct(node);
  if (left <= 10) return 'is-empty';
  if (left <= 30) return 'is-low';
  return 'is-plenty';
}

// Цвет несёт то же, что и слово: зелёная - нода раздаётся людям, красная -
// снята и никого не обслуживает, жёлтая - дорабатывает уже выданное
function stateClass(state) {
  if (state === 'active') return 'is-online';
  if (state === 'draining') return 'is-info';
  return 'is-offline';
}

function dotClass(state) {
  if (state === 'active') return 'is-live';
  if (state === 'draining') return 'is-draining';
  return 'is-down';
}

function stateLabel(state) {
  switch (state) {
    case 'active':
      return 'в выдаче';
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
