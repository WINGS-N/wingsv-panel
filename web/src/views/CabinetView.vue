<template>
  <div class="admin-shell">
    <PublicTopbar tag="Federation" />
    <main class="admin-main">
      <section class="surface-card">
        <div class="federation-live-head">
          <h2 class="section-title">Мой доступ</h2>
          <span v-if="trust" class="admin-pill" :class="bandClass(trust.band)">{{ bandLabel(trust.band) }}</span>
          <router-link v-if="hasPanel" class="cabinet-back" :to="{ name: 'admin-clients' }">
            Вернуться в панель
          </router-link>
        </div>
        <p class="body-copy body-copy-wide">
          Бесплатный доступ выдаётся из общего пула серверов, которые отдали администраторы. Сколько серверов
          достанется, решает уровень доверия аккаунта.
        </p>
        <p v-if="loadError" class="state-error">{{ loadError }}</p>
        <p v-else-if="!enabled" class="state-hint">Федерация выключена, выдавать пока нечего.</p>

        <div v-if="enabled" class="admin-stats">
          <div class="stat">
            <span class="stat-kicker">
              <Server :size="14" class="stat-kicker-icon" aria-hidden="true" />
              Серверов
            </span>
            <span class="stat-value">{{ access.nodes }}</span>
            <span class="stat-meta">{{ nodesMeta }}</span>
          </div>
          <div class="stat">
            <span class="stat-kicker">
              <Gauge :size="14" class="stat-kicker-icon" aria-hidden="true" />
              Скорость
            </span>
            <span class="stat-value">без лимита</span>
            <span class="stat-meta">упирается только в сам сервер</span>
          </div>
          <div class="stat">
            <span class="stat-kicker">
              <ArrowDownUp :size="14" class="stat-kicker-icon" aria-hidden="true" />
              Передано
            </span>
            <span class="stat-value">{{ formatBytes(access.used_bytes) }}</span>
            <span class="stat-meta">за текущий месяц</span>
          </div>
          <div class="stat">
            <span class="stat-kicker">
              <img src="/img/oneui/security-high.svg" alt="" class="stat-kicker-img" aria-hidden="true" />
              Доверие
            </span>
            <span class="stat-value">{{ trust ? trust.confidence : '-' }}</span>
            <span class="stat-meta">{{ trust ? bandMeaning(trust.band) : 'оценка появится с первой сессией' }}</span>
          </div>
        </div>

        <div v-if="enabled && access.subscription_url" class="entry-card mt-4">
          <p class="body-copy">
            Откройте на телефоне с установленным WINGS V - подписка заведётся сама и будет обновляться.
          </p>
          <div class="actions-row">
            <SamsungButton v-if="access.import_link" @click="openInApp">
              <template #icon><Smartphone class="button-icon" aria-hidden="true" /></template>
              Добавить в приложение
            </SamsungButton>
          </div>
          <p class="admin-muted mt-3">Или вставьте ссылку вручную:</p>
          <CopyableLink :value="access.subscription_url" class="mt-2" />
        </div>
      </section>

      <section class="surface-card mt-6">
        <h2 class="section-title">Аккаунт</h2>

        <h3 class="admin-section-subtitle mt-4">Пароль</h3>
        <p v-if="passwordOk" class="state-hint">Пароль сменён.</p>
        <p v-if="passwordError" class="state-error">{{ passwordError }}</p>
        <form class="form-grid mt-3" @submit.prevent="submitPassword">
          <OneuiInput v-model="passwords.old" label="Текущий пароль" type="password" autocomplete="current-password" />
          <OneuiInput v-model="passwords.next" label="Новый пароль" type="password" autocomplete="new-password" />
          <OneuiInput v-model="passwords.repeat" label="Повторите новый" type="password" autocomplete="new-password" />
        </form>
        <div class="actions-row mt-3">
          <SamsungButton :busy="passwordBusy" :disabled="!passwordReady" @click="submitPassword">
            Сменить пароль
          </SamsungButton>
        </div>

        <h3 class="admin-section-subtitle mt-6">Управление клиентами</h3>
        <template v-if="hasPanel">
          <p class="admin-muted">Панель вам открыта.</p>
          <div class="actions-row mt-3">
            <router-link class="cabinet-back" :to="{ name: 'admin-clients' }">Открыть панель</router-link>
          </div>
        </template>
        <template v-else>
          <p class="admin-muted">
            Своих клиентов заводят в админ-панели. Она открывается по решению владельца платформы.
          </p>
          <p v-if="panelError" class="state-error mt-2">{{ panelError }}</p>
          <p v-if="panelRequested" class="state-hint mt-2">Заявка отправлена, ждём решения владельца.</p>
          <div v-else class="actions-row mt-3">
            <SamsungButton :busy="panelBusy" @click="requestPanel">Запросить доступ к панели</SamsungButton>
          </div>
        </template>
      </section>

      <section class="surface-card mt-6">
        <h2 class="section-title">Приглашения</h2>
        <p class="admin-muted mt-1">
          Вход в федерацию только по коду. Приглашать могут те, кто сам отдал в неё сервер.
        </p>
        <p v-if="invites.reason" class="state-hint mt-2">{{ invites.reason }}</p>
        <p v-if="inviteError" class="state-error mt-2">{{ inviteError }}</p>
        <div v-if="invites.may_invite" class="actions-row mt-3">
          <SamsungButton :busy="inviteBusy" @click="createInvite">Создать код</SamsungButton>
        </div>
        <ul v-if="invites.list.length" class="cabinet-invites mt-4">
          <li v-for="it in invites.list" :key="it.token" class="cabinet-invite">
            <CopyableLink :value="inviteLink(it.token)" />
            <span class="admin-muted">{{ it.used_count || 0 }} из {{ it.max_uses || 1 }}</span>
          </li>
        </ul>
      </section>

      <section v-if="trust && trust.classes.length" class="surface-card mt-6">
        <h2 class="section-title">Что снижает доверие</h2>
        <p class="admin-muted mt-1">
          Считается только форма поведения: сколько адресов, откуда и какие порты. Что вы открываете, не смотрит никто.
        </p>
        <div class="fed-months mt-4">
          <div v-for="c in trust.classes" :key="c.kind" class="fed-month">
            <span class="fed-month-name">{{ classLabel(c.kind) }}</span>
            <span class="fed-month-track" aria-hidden="true">
              <span class="fed-month-fill" :style="{ width: classPct(c) + '%' }"></span>
            </span>
            <span class="fed-month-value">-{{ Math.round(c.weight) }}</span>
          </div>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { ArrowDownUp, Gauge, Server, Smartphone } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import PublicTopbar from '@/components/layout/PublicTopbar.vue';
import CopyableLink from '@/components/domain/CopyableLink.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import { changePassword, refreshSession } from '@/stores/auth.js';
import { authState } from '@/stores/auth.js';

const CLASS_LABELS = {
  high_fanout: 'много адресов сразу',
  geo_spread: 'работа из разных мест',
  port_scan: 'сканирование портов',
  mail_port: 'почтовые порты',
  torrent: 'торренты',
  malware: 'вредонос',
  upload_heavy: 'тяжёлая отдача',
  ads: 'реклама',
};

const enabled = ref(false);
const loadError = ref('');
const access = reactive({
  nodes: 0,
  nodes_entitled: 0,
  used_bytes: 0,
  subscription_url: '',
  sticky_until: 0,
  import_link: '',
});
const trustRaw = ref(null);
const trust = computed(() => trustRaw.value);
// Админ попадает сюда из своей же панели, и ему нужен путь обратно
const hasPanel = computed(() => Boolean(authState.value.admin?.panel_access));
// Когда выдано меньше положенного, дело не в доверии: две ноды одного донора
// падают вместе, поэтому вторую такую не дают
const nodesMeta = computed(() => {
  const entitled = Number(access.nodes_entitled || 0);
  if (entitled > access.nodes) {
    return `из ${entitled} по доверию - остальные пока некому отдать`;
  }
  return 'выдано вам сейчас';
});

onMounted(load);
onMounted(loadInvites);
onMounted(async () => {
  // Статус заявки живёт в сессии: кнопку нельзя показывать тому, кто уже попросил
  await refreshSession();
  panelRequested.value = Boolean(authState.value.admin?.panel_requested);
});

async function load() {
  try {
    const res = await fetch('/api/admin/me/access', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    enabled.value = Boolean(data.enabled);
    access.nodes = data.nodes || 0;
    access.subscription_url = data.subscription_url || '';
    access.sticky_until = data.sticky_until || 0;
    access.import_link = data.import_link || '';
    trustRaw.value = data.trust ? { ...data.trust, classes: data.trust.classes || [] } : null;
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  }
}

// Ссылка открывается своей схемой: приложение перехватывает её и заводит
// подписку, а браузер остаётся на месте
function openInApp() {
  window.location.href = access.import_link;
}

function classLabel(kind) {
  return CLASS_LABELS[kind] || kind;
}

function classPct(entry) {
  const top = Math.max(...(trust.value?.classes || []).map((c) => Number(c.weight) || 0), 1);
  return Math.max(2, Math.round((Number(entry.weight) / top) * 100));
}

function bandLabel(band) {
  if (band === 'full') return 'полный доступ';
  if (band === 'reduced') return 'урезанный доступ';
  if (band === 'quarantine') return 'карантин';
  return band;
}

const passwords = reactive({ old: '', next: '', repeat: '' });
const passwordBusy = ref(false);
const passwordError = ref('');
const passwordOk = ref(false);
const passwordReady = computed(
  () => passwords.old && passwords.next && passwords.next === passwords.repeat && !passwordBusy.value,
);

const panelBusy = ref(false);
const panelError = ref('');
const panelRequested = ref(false);

const invites = reactive({ list: [], may_invite: false, reason: '' });
const inviteBusy = ref(false);
const inviteError = ref('');

// Читается человеком, поэтому единицы английские, как везде в проекте
function formatBytes(value) {
  const bytes = Number(value || 0);
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let size = bytes / 1024;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(1)} ${units[unit]}`;
}

function inviteLink(token) {
  return `${window.location.origin}/register?invite=${token}`;
}

async function submitPassword() {
  if (!passwordReady.value) return;
  passwordBusy.value = true;
  passwordError.value = '';
  passwordOk.value = false;
  try {
    await changePassword(passwords.old, passwords.next);
    passwordOk.value = true;
    passwords.old = '';
    passwords.next = '';
    passwords.repeat = '';
  } catch (err) {
    passwordError.value = err.message || 'Не удалось сменить пароль';
  } finally {
    passwordBusy.value = false;
  }
}

async function requestPanel() {
  panelBusy.value = true;
  panelError.value = '';
  try {
    const res = await fetch('/api/admin/me/panel-request', { method: 'POST', credentials: 'include' });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.message || 'Не удалось отправить заявку');
    panelRequested.value = true;
  } catch (err) {
    panelError.value = err.message;
  } finally {
    panelBusy.value = false;
  }
}

async function loadInvites() {
  try {
    const res = await fetch('/api/admin/invites', { credentials: 'include' });
    if (!res.ok) return;
    const body = await res.json();
    invites.list = body.invites || [];
    invites.may_invite = Boolean(body.may_invite);
    invites.reason = body.reason || '';
  } catch {
    // Список приглашений - не то, ради чего стоит ронять весь кабинет
  }
}

async function createInvite() {
  inviteBusy.value = true;
  inviteError.value = '';
  try {
    const res = await fetch('/api/admin/invites', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.message || 'Не удалось создать код');
    await loadInvites();
  } catch (err) {
    inviteError.value = err.message;
  } finally {
    inviteBusy.value = false;
  }
}

function bandMeaning(band) {
  if (band === 'full') return 'несколько серверов на выбор';
  if (band === 'reduced') return 'один сервер и меньше скорость';
  return 'доступ приостановлен';
}

function bandClass(band) {
  if (band === 'full') return 'is-online';
  if (band === 'reduced') return 'is-info';
  return 'is-offline';
}
</script>

<style scoped>
.cabinet-back {
  margin-left: auto;
  font-size: 14px;
  color: rgba(252, 252, 252, 0.62);
  text-decoration: underline;
  text-underline-offset: 3px;
}

.cabinet-back:hover {
  color: #fbfbfb;
}

.cabinet-invites {
  display: grid;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.cabinet-invite {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}
</style>
