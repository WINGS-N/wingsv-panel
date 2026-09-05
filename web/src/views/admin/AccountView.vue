<template>
  <section class="admin-card">
    <!-- Шапка как в аккаунте телефона: аватар слева, имя рядом, загрузка под
         ним. Раздельные секции "Аватар" и "Имя" разносили одно и то же -->
    <div class="account-hero">
      <span class="account-hero-avatar" aria-hidden="true">
        <img :src="myAvatarUrl" alt="" />
      </span>
      <div class="account-hero-text">
        <h1 class="admin-card-title">{{ admin?.username || 'Аккаунт' }}</h1>
        <p class="admin-muted">{{ roleLabel }}</p>
        <p v-if="avatarError" class="admin-error mt-2">{{ avatarError }}</p>
      </div>

      <!-- Кнопки стоят в одной строке с именем, а не под ним со сдвигом -->
      <div class="avatar-actions">
        <input
          ref="fileInput"
          type="file"
          accept="image/png,image/jpeg,image/webp"
          class="hidden"
          @change="onFilePicked"
        />
        <SamsungButton variant="secondary" :busy="avatarBusy" @click="fileInput?.click()">
          <template #icon><Camera class="button-icon" aria-hidden="true" /></template>
          {{ avatarBusy ? 'Загружаем...' : 'Сменить фото' }}
        </SamsungButton>
        <SamsungButton v-if="hasCustomAvatar" variant="secondary" :disabled="avatarBusy" @click="removeAvatar">
          <template #icon><Trash2 class="button-icon" aria-hidden="true" /></template>
          Сбросить
        </SamsungButton>
      </div>
    </div>

    <div class="settings-grid mt-6">
      <div class="settings-card">
        <h2 class="admin-section-subtitle">Пароль</h2>
        <form class="admin-account-form" @submit.prevent="onSubmit">
          <OneuiInput v-model="oldPassword" label="Текущий пароль" type="password" autocomplete="current-password" />
          <div class="mt-3">
            <OneuiInput v-model="newPassword" label="Новый пароль" type="password" autocomplete="new-password" />
          </div>
          <div class="mt-3">
            <OneuiInput v-model="newPassword2" label="Повтор" type="password" autocomplete="new-password" />
          </div>
          <p v-if="error" class="admin-error mt-3">{{ error }}</p>
          <p v-if="ok" class="admin-success mt-3">Пароль обновлён.</p>
          <div class="actions-row mt-4">
            <SamsungButton type="submit" :busy="busy" :disabled="!canSubmit">
              <template #icon><KeyRound class="button-icon" aria-hidden="true" /></template>
              {{ busy ? 'Сохраняем...' : 'Сменить пароль' }}
            </SamsungButton>
          </div>
        </form>
      </div>

      <!-- Во всю ширину только когда внутри реально много: QR при настройке или
           список кодов. Пустая карточка на весь ряд выбивает соседей вниз -->
      <div class="settings-card">
        <h2 class="admin-section-subtitle">2FA</h2>
        <p class="admin-muted">
          Код из приложения-аутентификатора спрашивается при каждом входе - и в панели, и в приложении.
        </p>
        <p v-if="totpError" class="state-error mt-2">{{ totpError }}</p>

        <div v-if="totp.enabled">
          <div class="actions-row mt-3">
            <span class="admin-pill is-online">включён</span>
            <span class="admin-muted">кодов восстановления осталось: {{ totp.backup_codes }}</span>
            <SamsungButton v-if="!disarm.open && !reissue.open" variant="ghost" @click="reissue.open = true">
              Новые коды восстановления
            </SamsungButton>
            <SamsungButton v-if="!disarm.open && !reissue.open" variant="ghost" @click="disarm.open = true">
              Отключить
            </SamsungButton>
          </div>
          <!-- Новый набор кодов обесценивает старый, поэтому тоже под паролем -->
          <template v-if="reissue.open">
            <div class="form-grid mt-3">
              <OneuiInput
                v-model="reissue.password"
                label="Пароль от аккаунта"
                type="password"
                autocomplete="current-password"
              />
            </div>
            <div class="actions-row">
              <SamsungButton :busy="totpBusy" :disabled="!reissue.password" @click="reissueCodes">
                Выпустить коды
              </SamsungButton>
              <SamsungButton variant="ghost" @click="closeReissue">Отмена</SamsungButton>
            </div>
          </template>

          <!-- Снятие защиты подтверждается паролем, а не одним нажатием -->
          <template v-if="disarm.open">
            <div class="form-grid mt-3">
              <OneuiInput
                v-model="disarm.password"
                label="Пароль от аккаунта"
                type="password"
                autocomplete="current-password"
              />
            </div>
            <div class="actions-row">
              <SamsungButton :busy="totpBusy" :disabled="!disarm.password" @click="disableTotp">
                Отключить 2FA
              </SamsungButton>
              <SamsungButton variant="ghost" @click="closeDisarm">Отмена</SamsungButton>
            </div>
          </template>
        </div>

        <template v-else-if="totpSetup.otpauth">
          <p class="body-copy mt-3">
            Отсканируйте код приложением-аутентификатором и введите шесть цифр, которые оно покажет.
          </p>
          <img v-if="totpQr" :src="totpQr" alt="QR для 2FA" class="totp-qr" />
          <p class="admin-mono admin-muted">{{ totpSetup.secret }}</p>
          <div class="form-grid mt-3">
            <OneuiInput v-model.trim="totpCode" label="Код из приложения" inputmode="numeric" maxlength="6" />
          </div>
          <div class="actions-row">
            <SamsungButton :busy="totpBusy" @click="confirmTotp">Подтвердить</SamsungButton>
            <SamsungButton variant="ghost" @click="totpSetup.otpauth = ''">Отмена</SamsungButton>
          </div>
        </template>

        <div v-else class="actions-row mt-3">
          <SamsungButton :busy="totpBusy" @click="startTotp">
            <template #icon><ShieldCheck class="button-icon" aria-hidden="true" /></template>
            Включить
          </SamsungButton>
        </div>

        <!-- Коды показываются один раз: панель их не хранит в открытом виде -->
        <div v-if="backupCodes.length" class="entry-card mt-4">
          <p class="body-copy">
            Сохраните коды восстановления. Каждый работает один раз и нужен, когда телефона с аутентификатором нет под
            рукой.
          </p>
          <ul class="backup-codes mt-3">
            <li v-for="code in backupCodes" :key="code" class="admin-mono">{{ code }}</li>
          </ul>
        </div>
      </div>

      <div v-if="account.enabled" class="settings-card">
        <h2 class="admin-section-subtitle">{{ account.name }}</h2>
        <p class="admin-muted">Одна учётка на все наши сервисы. Пароль от панели при этом остаётся рабочим.</p>
        <p v-if="accountError" class="state-error mt-2">{{ accountError }}</p>
        <div class="actions-row mt-3">
          <template v-if="account.account">
            <span class="admin-mono">{{ account.account }}</span>
            <SamsungButton variant="ghost" :busy="accountBusy" @click="unlinkAccount">Отвязать</SamsungButton>
          </template>
          <SamsungButton v-else @click="linkAccount">Привязать учётку</SamsungButton>
        </div>
      </div>

      <!-- Панель открывается самому, если владелец не включил модерацию -->
      <div v-if="!hasPanel" class="settings-card">
        <h2 class="admin-section-subtitle">Управление клиентами</h2>
        <p class="admin-muted">Своих клиентов заводят в админ-панели. Откройте её себе, когда понадобится.</p>
        <p v-if="panelError" class="state-error mt-2">{{ panelError }}</p>
        <p v-if="panelRequested" class="state-hint mt-2">Заявка отправлена, ждём решения владельца.</p>
        <div v-else class="actions-row mt-3">
          <SamsungButton :busy="panelBusy" @click="openPanel">Стать администратором</SamsungButton>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Camera, KeyRound, ShieldCheck, Trash2 } from 'lucide-vue-next';
import { authState, changePassword, myAvatarUrl, refreshSession } from '@/stores/auth.js';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';

const totp = reactive({ enabled: false, pending: false, backup_codes: 0 });
const totpSetup = reactive({ secret: '', otpauth: '' });
const totpCode = ref('');
const totpBusy = ref(false);
const totpError = ref('');
const backupCodes = ref([]);
// Картинку рисует панель по своему же секрету: принимать её содержимое
// параметром значило бы рисовать чужой QR по чужой просьбе
const totpQr = computed(() => (totpSetup.otpauth ? `/api/admin/me/totp/qr?v=${totpVersion.value}` : ''));
const totpVersion = ref(0);

// Роль показывается словами: "owner" в интерфейсе ничего не объясняет
const roleLabel = computed(() => {
  if (admin.value?.role === 'owner') return 'Владелец платформы';
  return admin.value?.panel_access ? 'Администратор' : 'Участник федерации';
});

const account = reactive({ enabled: false, name: 'WINGS Account', account: '' });

const panelBusy = ref(false);
const panelError = ref('');
const panelRequested = ref(false);
const hasPanel = computed(() => Boolean(admin.value?.panel_access));

onMounted(() => {
  panelRequested.value = Boolean(admin.value?.panel_requested);
});

async function openPanel() {
  panelBusy.value = true;
  panelError.value = '';
  try {
    const res = await fetch('/api/admin/me/panel-access', { method: 'POST', credentials: 'include' });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.message || 'Не удалось открыть панель');
    // Когда владелец включил модерацию, доступ не выдаётся сразу
    panelRequested.value = Boolean(body.requested);
    if (body.granted) {
      await refreshSession();
      router.push({ name: 'admin-clients' });
    }
  } catch (err) {
    panelError.value = err.message;
  } finally {
    panelBusy.value = false;
  }
}
const accountBusy = ref(false);
const accountError = ref('');

const disarm = reactive({ open: false, password: '' });
const reissue = reactive({ open: false, password: '' });

onMounted(loadTotp);

async function loadTotp() {
  try {
    const res = await fetch('/api/admin/me/totp', { credentials: 'include' });
    if (res.ok) Object.assign(totp, await res.json());
  } catch {
    // Панель недоступна - состояние 2FA просто не показываем
  }
}

async function totpRequest(method, body) {
  const res = await fetch('/api/admin/me/totp', {
    method,
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.message || 'Не удалось');
  return data;
}

async function startTotp() {
  totpBusy.value = true;
  totpError.value = '';
  try {
    const data = await totpRequest('POST');
    totpSetup.secret = data.secret || '';
    totpSetup.otpauth = data.otpauth || '';
    totpCode.value = '';
    // QR рисует панель по своему секрету, и он только что сменился
    totpVersion.value += 1;
  } catch (err) {
    totpError.value = err.message;
  } finally {
    totpBusy.value = false;
  }
}

async function confirmTotp() {
  totpBusy.value = true;
  totpError.value = '';
  try {
    const data = await totpRequest('PUT', { code: totpCode.value });
    backupCodes.value = data.backup_codes || [];
    totpSetup.otpauth = '';
    totpSetup.secret = '';
    totpCode.value = '';
    await loadTotp();
  } catch (err) {
    totpError.value = err.message;
  } finally {
    totpBusy.value = false;
  }
}

async function disableTotp() {
  totpBusy.value = true;
  totpError.value = '';
  try {
    await totpRequest('DELETE', { password: disarm.password });
    backupCodes.value = [];
    closeDisarm();
    await loadTotp();
  } catch (err) {
    totpError.value = err.message;
  } finally {
    totpBusy.value = false;
  }
}

async function reissueCodes() {
  totpBusy.value = true;
  totpError.value = '';
  try {
    const data = await totpRequest('PATCH', { password: reissue.password });
    backupCodes.value = data.backup_codes || [];
    closeReissue();
    await loadTotp();
  } catch (err) {
    totpError.value = err.message;
  } finally {
    totpBusy.value = false;
  }
}

function closeReissue() {
  reissue.open = false;
  reissue.password = '';
}

function closeDisarm() {
  disarm.open = false;
  disarm.password = '';
}

onMounted(loadAccount);

async function loadAccount() {
  try {
    const res = await fetch('/api/oidc/link', { credentials: 'include' });
    if (res.ok) Object.assign(account, await res.json());
  } catch {
    // No account service configured is a normal state.
  }
}

// Привязка идёт через наш же экран переезда: там та же форма, и уводить
// человека на страницу провайдера незачем
function linkAccount() {
  router.push({ name: 'account-move' });
}

async function unlinkAccount() {
  accountBusy.value = true;
  accountError.value = '';
  try {
    const res = await fetch('/api/oidc/link', { method: 'DELETE', credentials: 'include' });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    await loadAccount();
  } catch (err) {
    accountError.value = String(err.message || err);
  } finally {
    accountBusy.value = false;
  }
}

const router = useRouter();
const admin = computed(() => authState.value.admin);
const oldPassword = ref('');
const newPassword = ref('');
const newPassword2 = ref('');
const busy = ref(false);
const error = ref('');
const ok = ref(false);

const fileInput = ref(null);
const avatarBusy = ref(false);
const avatarError = ref('');
const hasCustomAvatar = computed(() => (admin.value?.avatar_version || 0) > 0);

const canSubmit = computed(() => oldPassword.value && newPassword.value && newPassword.value === newPassword2.value);

async function onSubmit() {
  if (!canSubmit.value) {
    error.value = 'Новый пароль и повтор не совпадают';
    return;
  }
  busy.value = true;
  error.value = '';
  ok.value = false;
  try {
    await changePassword(oldPassword.value, newPassword.value);
    ok.value = true;
    oldPassword.value = '';
    newPassword.value = '';
    newPassword2.value = '';
  } catch (err) {
    error.value = err.message || 'Не удалось сменить пароль';
  } finally {
    busy.value = false;
  }
}

async function onFilePicked(e) {
  const file = e.target.files?.[0];
  e.target.value = '';
  if (!file) return;
  if (file.size > 2 * 1024 * 1024) {
    avatarError.value = 'Файл слишком большой (макс. 2 MB)';
    return;
  }
  avatarError.value = '';
  avatarBusy.value = true;
  try {
    const form = new FormData();
    form.append('avatar', file);
    const res = await fetch('/api/admin/me/avatar', {
      method: 'POST',
      credentials: 'include',
      body: form,
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || 'Не удалось загрузить');
    }
    await refreshSession();
  } catch (err) {
    avatarError.value = err.message;
  } finally {
    avatarBusy.value = false;
  }
}

async function removeAvatar() {
  avatarBusy.value = true;
  avatarError.value = '';
  try {
    await fetch('/api/admin/me/avatar', { method: 'DELETE', credentials: 'include' });
    await refreshSession();
  } catch (err) {
    avatarError.value = err.message;
  } finally {
    avatarBusy.value = false;
  }
}
</script>

<style scoped>
.avatar-row {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-top: 12px;
}

.avatar-preview {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 96px;
  height: 96px;
  border-radius: 9999px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.avatar-preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.avatar-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-left: auto;
}

.hidden {
  display: none;
}

.totp-qr {
  display: block;
  width: 200px;
  height: 200px;
  margin: 16px 0 12px;
  border-radius: 18px;
  background: #fbfbfb;
  padding: 10px;
}

.backup-codes {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}
</style>
