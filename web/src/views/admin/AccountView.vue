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
        <div class="avatar-actions mt-2">
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
        <p v-if="avatarError" class="admin-error mt-2">{{ avatarError }}</p>
      </div>
    </div>

    <h2 class="admin-section-subtitle mt-6">Пароль</h2>
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

    <h2 class="admin-section-subtitle mt-6">Второй фактор</h2>
    <p class="admin-muted">
      Код из приложения-аутентификатора спрашивается при каждом входе - и в панели, и в приложении. Пароль сам по себе
      входом перестаёт быть.
    </p>
    <p v-if="totpError" class="state-error mt-2">{{ totpError }}</p>

    <div v-if="totp.enabled" class="actions-row mt-3">
      <span class="admin-pill is-online">включён</span>
      <span class="admin-muted">резервных кодов осталось: {{ totp.backup_codes }}</span>
      <SamsungButton variant="ghost" :busy="totpBusy" @click="disableTotp">Отключить</SamsungButton>
    </div>

    <template v-else-if="totpSetup.otpauth">
      <p class="body-copy mt-3">
        Отсканируйте код приложением-аутентификатором и введите шесть цифр, которые оно покажет.
      </p>
      <img v-if="totpQr" :src="totpQr" alt="QR второго фактора" class="totp-qr" />
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
        Сохраните резервные коды. Каждый работает один раз и нужен, когда телефона с аутентификатором нет под рукой.
      </p>
      <ul class="backup-codes mt-3">
        <li v-for="code in backupCodes" :key="code" class="admin-mono">{{ code }}</li>
      </ul>
    </div>

    <template v-if="matrix.enabled">
      <h2 class="admin-section-subtitle mt-5">Matrix</h2>
      <p class="admin-muted">
        Вход через <strong>{{ matrix.homeserver }}</strong
        >. Аватар придётся загрузить здесь: аккаунт-сервис его не хранит, а профиль Matrix отдаёт картинку только вместе
        с доступом ко всей переписке — ради аватарки такое не берут.
      </p>
      <p v-if="matrixError" class="state-error mt-2">{{ matrixError }}</p>
      <div class="actions-row mt-3">
        <template v-if="matrix.matrix_id">
          <span class="admin-mono">{{ matrix.matrix_id }}</span>
          <SamsungButton variant="ghost" :busy="matrixBusy" @click="unlinkMatrix">Отвязать</SamsungButton>
        </template>
        <SamsungButton v-else @click="linkMatrix">Привязать аккаунт</SamsungButton>
      </div>
    </template>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
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

const matrix = reactive({ enabled: false, homeserver: '', matrix_id: '' });
const matrixBusy = ref(false);
const matrixError = ref('');

onMounted(loadTotp);

onMounted(loadMatrix);

async function loadMatrix() {
  try {
    const res = await fetch('/api/oidc/link', { credentials: 'include' });
    if (res.ok) Object.assign(matrix, await res.json());
  } catch {
    // No account service configured is a normal state.
  }
}

function linkMatrix() {
  window.location.href = `/api/oidc/start?return_to=${encodeURIComponent('/admin/account')}`;
}

async function unlinkMatrix() {
  matrixBusy.value = true;
  matrixError.value = '';
  try {
    const res = await fetch('/api/oidc/link', { method: 'DELETE', credentials: 'include' });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    await loadMatrix();
  } catch (err) {
    matrixError.value = String(err.message || err);
  } finally {
    matrixBusy.value = false;
  }
}

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
}

.hidden {
  display: none;
}
</style>
