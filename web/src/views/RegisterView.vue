<template>
  <!-- ============================================================
       REGISTER — same Samsung Account dialog vocabulary as Login,
       three underline fields (login / password / confirm password).
       ============================================================ -->
  <div class="login-stage">
    <header class="samsung-topbar">
      <router-link class="samsung-topbar-brand" to="/">
        <span class="wordmark-inline">WINGS V</span>
        <span class="samsung-topbar-divider">|</span>
        <span class="samsung-topbar-tag">Control Panel</span>
      </router-link>
    </header>

    <main class="login-main">
      <section class="login-card surface-card">
        <!-- Пришли по приглашению - это первое, что видно. Без карточки
             страница выглядит как обычная регистрация, и человек не понимает,
             куда и по чьей ссылке попал -->
        <div v-if="inviter.loaded && inviter.valid" class="invite-hero">
          <img :src="inviterAvatar" alt="" class="invite-avatar" aria-hidden="true" />
          <p class="invite-kicker">Вас пригласил</p>
          <p class="invite-name">{{ inviter.username || 'администратор' }}</p>
          <p class="invite-sub">
            Аккаунты здесь заводят только по приглашению.
            <template v-if="inviter.remaining > 1"> По этому коду осталось мест: {{ inviter.remaining }}.</template>
          </p>
        </div>
        <p v-else-if="inviter.loaded && !inviter.valid" class="state-error">
          {{ inviter.reason || 'Приглашение недействительно' }}
        </p>

        <h1 v-if="!inviter.valid" class="login-headline">
          <span>Создайте аккаунт.</span>
          <span>Одна учётная запись —</span>
          <span>контроль над несколькими устройствами.</span>
        </h1>
        <p class="login-sub">Заполните, чтобы начать</p>

        <form class="login-form" @submit.prevent="onSubmit">
          <div class="input-field">
            <OneuiInput
              v-model="username"
              label="Логин (только a-z, 0-9)"
              autocomplete="username"
              @update:model-value="onUsernameInput"
            />
          </div>

          <div class="input-field">
            <OneuiInput v-model="password" label="Пароль" type="password" autocomplete="new-password" />
          </div>

          <div class="input-field">
            <OneuiInput
              v-model="passwordConfirm"
              label="Повторите пароль"
              type="password"
              autocomplete="new-password"
            />
          </div>

          <div v-if="registrationState.mode === 'invite'" class="input-field">
            <OneuiInput v-model.trim="inviteToken" label="Invite-токен" autocomplete="off" />
          </div>

          <p v-if="registrationState.mode === 'closed'" class="state-error">
            Регистрация временно отключена администратором.
          </p>

          <p v-if="error" class="state-error">{{ error }}</p>

          <SamsungButton
            class="login-submit"
            type="submit"
            :busy="busy"
            :disabled="!canSubmit || registrationState.mode === 'closed'"
          >
            <template #icon><UserPlus class="button-icon" aria-hidden="true" /></template>
            {{ busy ? 'Создаём...' : 'Создать аккаунт' }}
          </SamsungButton>

          <SamsungButton
            v-if="matrix.enabled"
            variant="secondary"
            class="login-submit mt-3"
            type="button"
            :disabled="registrationState.mode === 'invite' && !inviteToken"
            @click="registerWithMatrix"
          >
            <template #icon><img src="/img/matrix.svg" alt="" class="button-icon-brand" aria-hidden="true" /></template>
            Создать по Matrix ID
          </SamsungButton>

          <!-- Код уезжает вместе с переходом: приглашают и тех, у кого аккаунт уже
               есть, - им он привяжет пригласившего вместо создания нового -->
          <router-link
            class="login-back-link"
            :to="{ name: 'login', query: inviteToken ? { invite: inviteToken } : {} }"
          >
            Уже есть аккаунт — войти
          </router-link>
        </form>
      </section>
    </main>

    <footer class="login-footer">
      <span class="wordmark-inline">WINGS V</span>
      <span class="login-footer-meta">WINGS-N · {{ year }} · All rights reserved</span>
    </footer>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { UserPlus } from 'lucide-vue-next';
import { register, registrationState } from '@/stores/auth.js';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';

const router = useRouter();
const username = ref('');
const password = ref('');
const passwordConfirm = ref('');
const inviteToken = ref(new URLSearchParams(window.location.search).get('invite') || '');
const inviter = reactive({
  loaded: false,
  valid: false,
  username: '',
  avatar_version: 0,
  admin_id: 0,
  reason: '',
  remaining: 0,
});

const inviterAvatar = computed(() =>
  inviter.avatar_version
    ? `/api/admin/avatars/${inviter.admin_id}.png?v=${inviter.avatar_version}`
    : '/img/avatar-default.png',
);

async function loadInviter() {
  if (!inviteToken.value) return;
  try {
    const res = await fetch(`/api/invite?invite=${encodeURIComponent(inviteToken.value)}`);
    if (!res.ok) return;
    Object.assign(inviter, await res.json(), { loaded: true });
  } catch {
    // Приглашение не удалось проверить - форма всё равно работает
  }
}
const matrix = reactive({ enabled: false, homeserver: '' });

// Код приглашения уходит в start, а не в callback: обратно провайдер вернёт
// только state и code, и без этого зарегистрировать первого посетителя нечем
onMounted(loadInviter);

onMounted(async () => {
  try {
    const res = await fetch('/api/oidc/status', { credentials: 'include' });
    if (res.ok) Object.assign(matrix, await res.json());
  } catch {
    // Аккаунт-сервис недоступен - просто не показываем кнопку
  }
});

function registerWithMatrix() {
  const params = new URLSearchParams({ return_to: '/admin/clients' });
  if (inviteToken.value) params.set('invite', inviteToken.value);
  window.location.href = `/api/oidc/start?${params.toString()}`;
}
const error = ref('');
const busy = ref(false);
const year = computed(() => new Date().getFullYear());

const canSubmit = computed(() => {
  if (!username.value || !password.value || !passwordConfirm.value) return false;
  if (registrationState.value.mode === 'invite' && !inviteToken.value) return false;
  return true;
});

function onUsernameInput(value) {
  // Force the input to the server-side username invariant (lowercase
  // alphanumeric only) so the user can't accidentally submit something the
  // backend will reject with a generic 400.
  const filtered = String(value ?? '')
    .toLowerCase()
    .replace(/[^a-z0-9]/g, '');
  if (filtered !== username.value) {
    username.value = filtered;
  }
}

async function onSubmit() {
  if (busy.value) return;
  error.value = '';
  if (password.value !== passwordConfirm.value) {
    error.value = 'Пароли не совпадают';
    return;
  }
  if (password.value.length < 8) {
    error.value = 'Пароль должен содержать не менее 8 символов';
    return;
  }
  busy.value = true;
  try {
    await register({
      username: username.value,
      password: password.value,
      inviteToken: inviteToken.value,
    });
    router.push('/admin/clients');
  } catch (err) {
    error.value = err.message || 'Не удалось создать аккаунт';
  } finally {
    busy.value = false;
  }
}
</script>

<style scoped>
.login-stage {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.login-main {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 24px;
}

.login-card {
  width: min(560px, 100%);
  padding: 56px 56px 64px;
}

@media (max-width: 640px) {
  .login-card {
    padding: 32px 22px 40px;
    border-radius: 22px;
  }
}

.login-headline {
  text-align: center;
  font-family: 'SamsungSharpSans', 'SamsungOne', sans-serif;
  font-weight: 700;
  font-size: clamp(22px, 2.8vw, 28px);
  line-height: 1.25;
  letter-spacing: -0.005em;
  color: #fbfbfb;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.login-sub {
  margin: 14px 0 0;
  text-align: center;
  font-size: 15px;
  color: rgba(252, 252, 252, 0.62);
}

.login-form {
  margin-top: 56px;
  display: flex;
  flex-direction: column;
  gap: 28px;
}

.login-submit {
  margin-top: 8px;
  width: 100%;
}

.login-back-link {
  margin-top: 6px;
  text-align: center;
  font-size: 14px;
  color: rgba(252, 252, 252, 0.78);
  text-decoration: underline;
  text-underline-offset: 3px;
}

.login-back-link:hover {
  color: #fbfbfb;
}

.login-footer {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 12px;
  padding: 24px 40px 32px;
  font-family: 'SamsungSharpSans', 'SamsungOne', sans-serif;
  font-size: 18px;
  color: rgba(252, 252, 252, 0.4);
}

.login-footer-meta {
  font-family: 'SamsungOne', sans-serif;
  font-size: 12px;
}

@media (max-width: 640px) {
  .login-footer {
    padding: 18px 20px 24px;
    font-size: 16px;
  }
}
</style>
