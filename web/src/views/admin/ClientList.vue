<template>
  <div class="dashboard">
    <section class="bento">
      <div class="bento-card bento-profile">
        <div class="profile-avatar">
          <img class="profile-avatar-img" :src="myAvatarUrl" alt="" />
          <span v-if="adminPresenceOnline" class="profile-dot" aria-hidden="true"></span>
          <router-link class="profile-avatar-edit" :to="{ name: 'admin-account' }" aria-label="Сменить аватар">
            <Camera class="h-4 w-4" aria-hidden="true" />
          </router-link>
        </div>
        <div class="profile-name wordmark-inline">{{ admin?.username || '—' }}</div>
        <div class="profile-handle">
          <span v-if="admin?.role === 'owner'">Владелец платформы</span>
          <span v-else>Администратор</span>
        </div>
      </div>

      <div v-if="admin?.must_change_password" class="bento-card bento-warn">
        <span class="bento-icon bento-icon-img">
          <img src="/img/icon-warn.png" alt="" />
        </span>
        <div>
          <div class="bento-kicker">Используется пароль по умолчанию</div>
          <p class="bento-text">
            Смените пароль администратора, чтобы защитить доступ к панели.
            <router-link :to="{ name: 'admin-account' }">Перейти в «Аккаунт»</router-link>
          </p>
        </div>
      </div>

      <div class="bento-card bento-privacy">
        <span class="bento-icon bento-icon-img">
          <img src="/img/icon-pin.png" alt="" />
        </span>
        <div>
          <div class="bento-kicker">Доступ</div>
          <p class="bento-text">
            На каждого клиента панель генерирует <code class="font-sharp">wingsv://</code> ссылку со встроенным токеном.
            Откройте её в WINGS V на устройстве — клиент привяжется к панели и начнёт получать настройки. Открыть ссылку
            повторно можно из карточки клиента.
          </p>
        </div>
      </div>

      <div v-if="!admin?.must_change_password" class="bento-card">
        <span class="bento-icon bento-icon-img">
          <img src="/img/icon-shield.png" alt="" />
        </span>
        <div>
          <div class="bento-kicker">Внимание</div>
          <p class="bento-text">
            Конфигурации применяются на устройства мгновенно — пуш доходит до подключённого клиента за секунды. Меняйте
            настройки осторожно: ошибочное правило или выключенный backend могут оборвать связь у пользователя.
          </p>
        </div>
      </div>

      <div class="bento-card bento-services">
        <div class="bento-row">
          <span class="bento-icon bento-icon-img">
            <img src="/img/icon-phone.png" alt="" />
          </span>
          <div class="bento-title-stack">
            <div class="bento-card-title">Клиенты</div>
          </div>
        </div>
        <div class="devices-numbers">
          <div class="devices-number">
            <span class="devices-number-value">{{ clients.length }}</span>
            <span class="devices-number-label">всего</span>
          </div>
          <div class="devices-number">
            <span class="devices-number-value" style="color: #6fe3a4">{{ onlineCount }}</span>
            <span class="devices-number-label">онлайн</span>
          </div>
        </div>
        <div class="actions-row mt-4">
          <SamsungButton @click="scrollToTable">Открыть таблицу</SamsungButton>
          <SamsungButton variant="secondary" :busy="creating" @click="openCreate">
            <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
            {{ creating ? 'Создаём…' : 'Новый клиент' }}
          </SamsungButton>
        </div>
      </div>
    </section>

    <section ref="tableSection" class="admin-card mt-6">
      <header class="admin-card-header">
        <div>
          <h1 class="admin-card-title">Клиенты</h1>
          <p class="body-copy">Управляйте конфигурациями и удалённо переключайте состояния клиентов.</p>
        </div>
        <SamsungButton :busy="creating" @click="openCreate">
          <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
          {{ creating ? 'Создаём…' : 'Новый клиент' }}
        </SamsungButton>
      </header>

      <div class="mt-2 client-filters">
        <OneuiRadioGroup v-model="managementFilter" :options="managementPillOptions" variant="pill" />
        <OneuiRadioGroup v-model="statusFilter" :options="statusPillOptions" variant="pill" />
      </div>

      <p v-if="loadError" class="admin-error">{{ loadError }}</p>
      <SamsungSectionLoader v-else-if="loading && clients.length === 0" />

      <table v-if="paginatedClients.length" class="admin-table">
        <thead>
          <tr>
            <th>
              <button class="th-sort" type="button" @click="setSort('name')">Имя {{ sortIndicator('name') }}</button>
            </th>
            <th>
              <button class="th-sort" type="button" @click="setSort('backend')">
                Бэкенд {{ sortIndicator('backend') }}
              </button>
            </th>
            <th>Устройство</th>
            <th>OS / WINGS V</th>
            <th>
              <button class="th-sort" type="button" @click="setSort('traffic')">
                Трафик {{ sortIndicator('traffic') }}
              </button>
            </th>
            <th>
              <button class="th-sort" type="button" @click="setSort('status')">
                Статус {{ sortIndicator('status') }}
              </button>
            </th>
            <th>
              <button class="th-sort" type="button" @click="setSort('last_seen')">
                Последний контакт {{ sortIndicator('last_seen') }}
              </button>
            </th>
            <th class="admin-row-actions-col" aria-label=""></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="client in paginatedClients" :key="client.id" class="admin-row" @click="openClient(client.id)">
            <td data-label="Имя">
              <div class="flex items-center gap-3">
                <span class="client-avatar">{{ initialsOf(client.name || client.id) }}</span>
                <div>
                  <div class="admin-row-name">{{ client.name }}</div>
                  <code class="admin-mono">{{ client.id }}</code>
                </div>
              </div>
            </td>
            <td data-label="Бэкенд">{{ client.backend_type || '—' }}</td>
            <td data-label="Устройство">{{ client.device_model || '—' }}</td>
            <td data-label="OS / WINGS V">{{ client.os_version || '—' }} · {{ client.app_version || '—' }}</td>
            <td data-label="Трафик">
              <div v-if="trafficHasLimit(client)" class="traffic-cell">
                <div class="traffic-usage">
                  <span :class="{ 'traffic-over': trafficPercent(client) >= 100 }">{{
                    formatBytes(trafficUsedBytes(client))
                  }}</span>
                  <span class="traffic-sep">/</span>
                  <span class="traffic-limit">{{ formatBytes(trafficLimitBytes(client)) }}</span>
                </div>
                <div class="traffic-bar">
                  <div
                    class="traffic-bar-fill"
                    :class="{ 'is-full': trafficPercent(client) >= 100 }"
                    :style="{ width: trafficPercent(client) + '%' }"
                  ></div>
                </div>
              </div>
              <template v-else>{{ trafficText(client) }}</template>
            </td>
            <td data-label="Статус">
              <SamsungPill :variant="client.online ? 'online' : 'offline'">
                {{ client.online ? 'Онлайн' : 'Оффлайн' }}
              </SamsungPill>
            </td>
            <td data-label="Контакт">{{ formatTs(client.last_seen_at) }}</td>
            <td class="admin-row-actions" @click.stop>
              <div class="admin-row-actions-inner">
                <OneuiSwitch
                  v-if="client.managed"
                  :model-value="!client.disabled"
                  :disabled="togglingId === client.id"
                  :aria-label="client.disabled ? `Включить ${client.name}` : `Выключить ${client.name}`"
                  @change="toggleDisabled(client, $event)"
                />
                <SamsungIconButton
                  v-if="trafficHasLimit(client)"
                  variant="secondary"
                  :busy="resettingId === client.id"
                  :aria-label="`Сбросить трафик ${client.name}`"
                  @click.stop="resetTraffic(client)"
                >
                  <RotateCcw class="h-4 w-4" aria-hidden="true" />
                </SamsungIconButton>
                <SamsungIconButton
                  variant="danger"
                  :busy="deletingId === client.id"
                  :aria-label="`Удалить ${client.name}`"
                  @click.stop="askDelete(client)"
                >
                  <Trash2 class="h-4 w-4" aria-hidden="true" />
                </SamsungIconButton>
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="filteredClients.length > pageSize" class="admin-pagination">
        <SamsungButton variant="secondary" :disabled="page === 0" @click="page = Math.max(0, page - 1)">
          <template #icon><ChevronLeft class="button-icon" aria-hidden="true" /></template>
          Назад
        </SamsungButton>
        <span class="admin-muted">
          {{ page * pageSize + 1 }}–{{ Math.min((page + 1) * pageSize, filteredClients.length) }} из
          {{ filteredClients.length }}
        </span>
        <SamsungButton
          variant="secondary"
          :disabled="(page + 1) * pageSize >= filteredClients.length"
          @click="page += 1"
        >
          Вперёд
          <ChevronRight class="button-icon" aria-hidden="true" />
        </SamsungButton>
      </div>

      <p v-if="!loading && clients.length === 0" class="admin-muted">
        Пока нет ни одного клиента. Нажмите «Новый клиент», чтобы добавить.
      </p>
      <p v-else-if="!loading && filteredClients.length === 0" class="admin-muted">В этой выборке нет клиентов.</p>

      <SamsungModal v-model="showCreate" :busy="creating" title="Новый клиент">
        <p class="body-copy">
          После создания вы получите ссылку <code class="font-sharp">wingsv://</code> со встроенным токеном — её нужно
          открыть в WINGS V на устройстве клиента. Ссылку можно посмотреть позже на странице клиента.
        </p>
        <OneuiInput v-model.trim="newName" label="Имя клиента" placeholder="Например, телефон Никиты" class="mt-4" />

        <label class="field-label mt-4">Тип управления</label>
        <OneuiRadioGroup v-model="managementType" :options="managementTypeOptions" variant="pill" />
        <p class="admin-muted remote-toggle-hint">
          Полный — панель управляет конфигурацией удалённо (Guardian, WSS-контроль). Только выдача конфигурации — панель
          лишь выдаёт wg-конфиг по токену, без удалённого контроля.
        </p>

        <template v-if="managementType === 'full'">
          <label class="field-label mt-4">Режим синхронизации</label>
          <OneuiRadioGroup v-model="syncMode" :options="syncModeOptions" variant="pill" />
          <div v-if="syncMode === 'periodic'" class="form-row form-row-stack mt-2">
            <OneuiInput
              v-model.number="syncIntervalMinutes"
              label="Интервал (мин, минимум 15)"
              type="number"
              :min="15"
              narrow
            />
          </div>
        </template>

        <template v-if="vkTurnNodeOptions.length">
          <label class="field-label mt-4">VK TURN сервер</label>
          <p class="admin-muted remote-toggle-hint">
            Реле, к которому клиент подключится и автоматически выпустит wg-конфиг. «Не выбран» — без автоматической wg.
          </p>
          <OneuiSelect
            :model-value="vkTurnNodeId"
            :options="vkTurnNodeOptions"
            @update:model-value="vkTurnNodeId = $event"
          />
          <template v-if="vkTurnNodeId">
            <div class="form-row form-row-stack mt-3">
              <OneuiInput
                v-model.number="trafficLimitGb"
                label="Лимит трафика (ГБ, 0 — без лимита)"
                type="number"
                :min="0"
              />
            </div>
            <div class="form-row form-row-stack mt-2">
              <OneuiInput
                v-model.number="resetPeriodDays"
                label="Сброс каждые N дней (0 — только вручную)"
                type="number"
                :min="0"
              />
            </div>
          </template>
        </template>

        <template v-if="managementType === 'full'">
          <label class="field-label mt-4">Заполнение конфигурации</label>
          <OneuiRadioGroup v-model="seedMode" :options="seedModeOptions" variant="pill" />

          <template v-if="seedMode === 'clone'">
            <label class="field-label mt-3">Источник</label>
            <OneuiSelect
              :model-value="seedFromClientId"
              :options="seedClientOptions"
              @update:model-value="seedFromClientId = $event"
            />
          </template>
          <template v-else-if="seedMode === 'link'">
            <OneuiTextarea
              v-model.trim="seedLink"
              label="Ссылка (wingsv:// или vless://)"
              rows="3"
              placeholder="wingsv://... или vless://..."
              class="mt-3"
            />
          </template>
        </template>

        <p v-if="createError" class="admin-error mt-3">{{ createError }}</p>
        <template #actions>
          <SamsungButton :busy="creating" :disabled="!canCreate" @click="createClient">
            <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
            {{ creating ? 'Создаём…' : 'Создать' }}
          </SamsungButton>
          <SamsungButton variant="secondary" :disabled="creating" @click="closeCreate">
            <template #icon><X class="button-icon" aria-hidden="true" /></template>
            Отмена
          </SamsungButton>
        </template>
      </SamsungModal>

      <SamsungModal
        :model-value="!!confirmDelete"
        :busy="!!deletingId"
        title="Удалить клиента?"
        @update:model-value="cancelDelete"
      >
        <p v-if="confirmDelete" class="body-copy">
          Клиент <strong>{{ confirmDelete.name }}</strong> будет удалён без возможности восстановления — токен
          перестанет работать, конфигурация и журналы исчезнут. Само устройство выйдет из-под управления панели.
        </p>
        <p v-if="deleteError" class="admin-error mt-3">{{ deleteError }}</p>
        <template #actions>
          <SamsungButton variant="danger" :busy="deletingId === confirmDelete?.id" @click="performDelete">
            <template #icon><Trash2 class="button-icon" aria-hidden="true" /></template>
            {{ deletingId === confirmDelete?.id ? 'Удаляем…' : 'Удалить' }}
          </SamsungButton>
          <SamsungButton variant="secondary" :disabled="deletingId === confirmDelete?.id" @click="cancelDelete">
            <template #icon><X class="button-icon" aria-hidden="true" /></template>
            Отмена
          </SamsungButton>
        </template>
      </SamsungModal>

      <SamsungModal :model-value="!!lastCreatedLink" title="Клиент создан" @update:model-value="dismissLink">
        <p class="body-copy">Откройте ссылку в WINGS V на устройстве клиента:</p>
        <CopyableLink :value="lastCreatedLink" rows="3" />
        <div v-if="createdQr" class="created-qr mt-3">
          <img :src="createdQr" alt="QR-код ссылки" width="240" height="240" />
        </div>
        <p class="admin-muted mt-3">
          Ссылку можно посмотреть позже — откройте клиента в списке и нажмите «Показать ссылку».
        </p>
        <template #actions>
          <SamsungButton @click="dismissLink">
            <template #icon><X class="button-icon" aria-hidden="true" /></template>
            Закрыть
          </SamsungButton>
        </template>
      </SamsungModal>
    </section>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { Camera, ChevronLeft, ChevronRight, Plus, RotateCcw, Trash2, X } from 'lucide-vue-next';
import { authState, myAvatarUrl } from '@/stores/auth.js';
import { connectAdminSocket } from '@/stores/admin-socket.js';
import { formatBytes } from '@/utils/format.js';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import OneuiRadioGroup from '@/components/controls/OneuiRadioGroup.vue';
import OneuiSelect from '@/components/controls/OneuiSelect.vue';
import OneuiSwitch from '@/components/controls/OneuiSwitch.vue';
import OneuiTextarea from '@/components/controls/OneuiTextarea.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungIconButton from '@/components/layout/SamsungIconButton.vue';
import SamsungModal from '@/components/layout/SamsungModal.vue';
import SamsungPill from '@/components/layout/SamsungPill.vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';
import CopyableLink from '@/components/domain/CopyableLink.vue';

function clientTrafficTotal(client) {
  // Traffic is wg-peer accounting; it only means anything for a provisioned VK
  // TURN client. Non-provisioned clients count as zero for sort purposes.
  if (!client.provisioned) return 0;
  return (Number(client.traffic_rx) || 0) + (Number(client.traffic_tx) || 0);
}

function trafficText(client) {
  if (!client.provisioned) return '—';
  const total = clientTrafficTotal(client);
  return total > 0 ? formatBytes(total) : '—';
}

function trafficLimitBytes(client) {
  return client.managed ? Number(client.traffic_limit_bytes) || 0 : 0;
}

function trafficHasLimit(client) {
  return trafficLimitBytes(client) > 0;
}

function trafficUsedBytes(client) {
  return Number(client.traffic_used_bytes) || 0;
}

// trafficPercent is the share of the cap consumed, capped at 100 for the bar.
function trafficPercent(client) {
  const limit = trafficLimitBytes(client);
  if (limit <= 0) return 0;
  return Math.min(100, Math.round((trafficUsedBytes(client) / limit) * 100));
}

const router = useRouter();
const clients = ref([]);
const loading = ref(false);
const loadError = ref('');
const tableSection = ref(null);
const statusFilter = ref('all');
const managementFilter = ref('all');
const admin = computed(() => authState.value.admin);
const adminPresenceOnline = computed(() => clients.value.some((c) => c.online));
const onlineCount = computed(() => clients.value.filter((c) => c.online).length);
const offlineCount = computed(() => clients.value.length - onlineCount.value);
const fullControlCount = computed(() => clients.value.filter((c) => c.remote_control).length);
const configOnlyCount = computed(() => clients.value.length - fullControlCount.value);

// Management-type filter sits above the online/offline pills: it is the coarser
// grouping (how the panel manages the client), so pick it first.
const managementPillOptions = computed(() => [
  { value: 'all', label: 'Все', count: clients.value.length },
  { value: 'full', label: 'Полный контроль', count: fullControlCount.value },
  { value: 'config', label: 'Только конфигурация', count: configOnlyCount.value },
]);

const statusPillOptions = computed(() => [
  { value: 'all', label: 'Все', count: clients.value.length },
  { value: 'online', label: 'Онлайн', count: onlineCount.value },
  { value: 'offline', label: 'Оффлайн', count: offlineCount.value },
]);

const sortKey = ref('last_seen');
const sortDir = ref('desc');
const page = ref(0);
const pageSize = 20;

function setSort(key) {
  if (sortKey.value === key) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc';
  } else {
    sortKey.value = key;
    sortDir.value = key === 'name' ? 'asc' : 'desc';
  }
  page.value = 0;
}

function sortIndicator(key) {
  if (sortKey.value !== key) return '';
  return sortDir.value === 'asc' ? '↑' : '↓';
}

function initialsOf(value) {
  const v = String(value || '').trim();
  if (!v) return '·';
  const parts = v.split(/[\s._-]+/).filter(Boolean);
  if (parts.length === 0) return v.slice(0, 2).toUpperCase();
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}

const filteredClients = computed(() => {
  let list = [...clients.value];
  if (managementFilter.value === 'full') list = list.filter((c) => c.remote_control);
  else if (managementFilter.value === 'config') list = list.filter((c) => !c.remote_control);
  if (statusFilter.value === 'online') list = list.filter((c) => c.online);
  else if (statusFilter.value === 'offline') list = list.filter((c) => !c.online);
  const dir = sortDir.value === 'asc' ? 1 : -1;
  const cmp = (a, b) => {
    switch (sortKey.value) {
      case 'name':
        return (a.name || '').localeCompare(b.name || '') * dir;
      case 'backend':
        return (a.backend_type || '').localeCompare(b.backend_type || '') * dir;
      case 'status':
        return (Number(a.online) - Number(b.online)) * dir;
      case 'traffic':
        return (clientTrafficTotal(a) - clientTrafficTotal(b)) * dir;
      case 'last_seen':
        return ((new Date(a.last_seen_at).valueOf() || 0) - (new Date(b.last_seen_at).valueOf() || 0)) * dir;
      default:
        return 0;
    }
  };
  return list.sort(cmp);
});

const paginatedClients = computed(() => {
  const start = page.value * pageSize;
  return filteredClients.value.slice(start, start + pageSize);
});

watch([managementFilter, statusFilter], () => {
  page.value = 0;
});

function scrollToTable() {
  tableSection.value?.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

const showCreate = ref(false);
const newName = ref('');
const managementType = ref('full');
const managementTypeOptions = [
  { value: 'full', label: 'Полный' },
  { value: 'config', label: 'Только выдача конфигурации VK TURN' },
];
const vkTurnNodes = ref([]);
const vkTurnNodeId = ref('');
const trafficLimitGb = ref('');
const resetPeriodDays = ref('');
const vkTurnNodeOptions = computed(() => [
  { value: '', label: 'Не выбран' },
  ...vkTurnNodes.value.map((n) => ({ value: n.id, label: n.name || n.id })),
]);
const creating = ref(false);
const createError = ref('');
const lastCreatedLink = ref('');
const createdQr = ref('');
const seedMode = ref('empty');
const seedFromClientId = ref('');
const seedLink = ref('');
const syncMode = ref('periodic');
const syncIntervalMinutes = ref(15);
const confirmDelete = ref(null);
const deletingId = ref('');
const deleteError = ref('');
const togglingId = ref('');
const resettingId = ref('');

const canCreate = computed(() => {
  if (!newName.value) return false;
  if (seedMode.value === 'clone' && !seedFromClientId.value) return false;
  if (seedMode.value === 'link' && !seedLink.value) return false;
  return true;
});

const seedClientOptions = computed(() => [
  { value: '', label: '— выберите клиента —' },
  ...clients.value.map((c) => ({ value: c.id, label: `${c.name} · ${c.id}` })),
]);

const syncModeOptions = [
  { value: 'always', label: 'Всегда в фоне' },
  { value: 'periodic', label: 'Периодически' },
  { value: 'foreground', label: 'Только пока приложение открыто' },
];

const seedModeOptions = computed(() => [
  { value: 'empty', label: 'Пустая' },
  { value: 'clone', label: 'Скопировать с другого клиента', disabled: clients.value.length === 0 },
  { value: 'link', label: 'Из wingsv:// / vless://' },
]);

let socketHandle = null;

async function loadClients() {
  loading.value = true;
  loadError.value = '';
  try {
    const res = await fetch('/api/admin/clients', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    clients.value = data.clients || [];
  } catch (err) {
    loadError.value = err.message || 'Не удалось загрузить список';
  } finally {
    loading.value = false;
  }
}

async function loadVkTurnNodes() {
  try {
    const res = await fetch('/api/admin/nodes', { credentials: 'include' });
    if (!res.ok) return;
    const data = await res.json();
    vkTurnNodes.value = (data.nodes || []).filter((n) => n.kind === 'vk_turn_proxy');
  } catch {
    // Non-fatal: without nodes the form falls back to the configured endpoint.
  }
}

function openCreate() {
  showCreate.value = true;
  newName.value = '';
  managementType.value = 'full';
  vkTurnNodeId.value = '';
  createError.value = '';
  lastCreatedLink.value = '';
  createdQr.value = '';
  seedMode.value = 'empty';
  seedFromClientId.value = '';
  seedLink.value = '';
  syncMode.value = 'periodic';
  syncIntervalMinutes.value = 15;
}

function closeCreate() {
  showCreate.value = false;
}

function dismissLink() {
  lastCreatedLink.value = '';
  createdQr.value = '';
}

async function generateQR(link) {
  if (!link) return '';
  try {
    const QR = await import('qrcode');
    const canvas = document.createElement('canvas');
    canvas.width = 320;
    canvas.height = 320;
    await QR.toCanvas(canvas, link, {
      errorCorrectionLevel: 'H',
      width: 320,
      margin: 1,
      color: { dark: '#000000', light: '#ffffff' },
    });
    // Overlay WINGS V app icon in the center.
    const ctx = canvas.getContext('2d');
    const icon = new Image();
    icon.src = '/img/wingsv-icon.webp';
    await new Promise((resolve) => {
      icon.onload = resolve;
      icon.onerror = resolve;
    });
    const size = 64;
    const x = (canvas.width - size) / 2;
    const y = (canvas.height - size) / 2;
    ctx.fillStyle = '#ffffff';
    ctx.beginPath();
    ctx.roundRect(x - 6, y - 6, size + 12, size + 12, 16);
    ctx.fill();
    ctx.save();
    ctx.beginPath();
    ctx.roundRect(x, y, size, size, 14);
    ctx.clip();
    ctx.drawImage(icon, x, y, size, size);
    ctx.restore();
    return canvas.toDataURL('image/png');
  } catch {
    return '';
  }
}

async function createClient() {
  if (!canCreate.value || creating.value) return;
  creating.value = true;
  createError.value = '';
  try {
    const reqBody = {
      name: newName.value,
      sync_mode: syncMode.value,
      periodic_interval_minutes: Math.max(15, Number(syncIntervalMinutes.value) || 30),
      remote_control: managementType.value === 'full',
    };
    if (vkTurnNodeId.value) {
      reqBody.vk_turn_node_id = vkTurnNodeId.value;
      const gb = Number(trafficLimitGb.value) || 0;
      const days = Number(resetPeriodDays.value) || 0;
      if (gb > 0) reqBody.traffic_limit_gb = gb;
      if (days > 0) reqBody.reset_period_days = days;
    }
    if (seedMode.value === 'clone' && seedFromClientId.value) {
      reqBody.seed_from_client_id = seedFromClientId.value;
    } else if (seedMode.value === 'link' && seedLink.value) {
      reqBody.seed_from_wingsv_link = seedLink.value;
    }
    const res = await fetch('/api/admin/clients', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(reqBody),
    });
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}));
      throw new Error(errBody.message || 'Не удалось создать клиента');
    }
    const respBody = await res.json();
    showCreate.value = false;
    lastCreatedLink.value = respBody.wingsv_link;
    createdQr.value = await generateQR(respBody.wingsv_link);
    await loadClients();
  } catch (err) {
    createError.value = err.message || 'Не удалось создать клиента';
  } finally {
    creating.value = false;
  }
}

function openClient(id) {
  router.push({ name: 'admin-client-detail', params: { id } });
}

function askDelete(client) {
  confirmDelete.value = { id: client.id, name: client.name || client.id };
  deleteError.value = '';
}

function cancelDelete() {
  if (deletingId.value) return;
  confirmDelete.value = null;
  deleteError.value = '';
}

async function performDelete() {
  if (!confirmDelete.value) return;
  const target = confirmDelete.value;
  deletingId.value = target.id;
  deleteError.value = '';
  try {
    const res = await fetch(`/api/admin/clients/${encodeURIComponent(target.id)}`, {
      method: 'DELETE',
      credentials: 'include',
    });
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}));
      throw new Error(errBody.message || 'Не удалось удалить клиента');
    }
    confirmDelete.value = null;
    await loadClients();
  } catch (err) {
    deleteError.value = err.message || 'Не удалось удалить клиента';
  } finally {
    deletingId.value = '';
  }
}

// toggleDisabled flips a managed client's cutoff. The switch shows enabled state,
// so a checked switch means disabled=false. On failure we resync from the server.
async function toggleDisabled(client, enabled) {
  if (togglingId.value) return;
  togglingId.value = client.id;
  try {
    const res = await fetch(`/api/admin/clients/${encodeURIComponent(client.id)}/disabled`, {
      method: 'PUT',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ disabled: !enabled }),
    });
    if (!res.ok) throw new Error('toggle failed');
    client.disabled = !enabled;
  } catch {
    await loadClients();
  } finally {
    togglingId.value = '';
  }
}

async function resetTraffic(client) {
  if (resettingId.value) return;
  resettingId.value = client.id;
  try {
    const res = await fetch(`/api/admin/clients/${encodeURIComponent(client.id)}/traffic-reset`, {
      method: 'POST',
      credentials: 'include',
    });
    if (!res.ok) throw new Error('reset failed');
    await loadClients();
  } catch {
    // A failed reset leaves usage as-is; nothing to roll back.
  } finally {
    resettingId.value = '';
  }
}

function formatTs(iso) {
  if (!iso || iso.startsWith('1970')) return '—';
  try {
    return new Date(iso).toLocaleString('ru-RU');
  } catch (err) {
    return iso;
  }
}

onMounted(() => {
  loadClients();
  loadVkTurnNodes();
  socketHandle = connectAdminSocket((event) => {
    if (event.kind === 'status_update' || event.kind === 'error') {
      // Re-fetch list on presence-related events; cheaper than diffing.
      loadClients();
    }
  });
});

onBeforeUnmount(() => {
  if (socketHandle) socketHandle.close();
});
</script>

<style scoped>
.client-filters {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.remote-toggle-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}
.remote-toggle-hint {
  margin-top: 4px;
  max-width: 42ch;
}
.created-qr {
  display: flex;
  justify-content: center;
}
.created-qr img {
  border-radius: 12px;
}
.traffic-cell {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 92px;
}
.traffic-usage {
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.traffic-sep {
  opacity: 0.45;
  margin: 0 3px;
}
.traffic-limit {
  opacity: 0.7;
}
.traffic-over {
  color: #e5484d;
  font-weight: 600;
}
.traffic-bar {
  height: 4px;
  border-radius: 999px;
  background: rgba(128, 128, 128, 0.22);
  overflow: hidden;
}
.traffic-bar-fill {
  height: 100%;
  border-radius: 999px;
  background: #4b8bf5;
  transition: width 0.3s ease;
}
.traffic-bar-fill.is-full {
  background: #e5484d;
}
.admin-row-actions-inner {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}
</style>
