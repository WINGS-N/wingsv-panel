<template>
  <!-- ============================================================
       LOGIN — centered Samsung Account-style dialog card.
       Two big-typography lines as headline, sub-line in muted text,
       underline inputs, solid-blue pill primary, neutral secondary.
       ============================================================ -->
  <div class="login-stage">
    <header class="samsung-topbar">
      <router-link class="samsung-topbar-brand" to="/">
        <span class="wordmark-inline">WINGS V</span>
        <span class="samsung-topbar-divider">|</span>
        <span class="samsung-topbar-tag">{{ inviteToken ? 'Federation' : 'Control Panel' }}</span>
      </router-link>
    </header>

    <main class="login-main">
      <section class="login-card surface-card">
        <InviteHero :token="inviteToken" />

        <h1 class="login-headline">
          <span>Один аккаунт.</span>
          <span>Любое устройство.</span>
          <span>Только для вас.</span>
        </h1>
        <p class="login-sub">Войдите для начала</p>

        <form class="login-form" @submit.prevent="onSubmit">
          <div class="input-field">
            <OneuiInput v-model.trim="username" label="Логин" autocomplete="username" />
          </div>

          <div class="input-field">
            <OneuiInput v-model="password" label="Пароль" type="password" autocomplete="current-password" />
          </div>

          <!-- Поле кода появляется, только когда панель его спросила -->
          <div v-if="needsTotp" class="input-field">
            <OneuiInput
              v-model.trim="totpCode"
              label="Код из аутентификатора"
              inputmode="numeric"
              autocomplete="one-time-code"
            />
          </div>

          <p v-if="error" class="state-error">{{ error }}</p>

          <SamsungButton class="login-submit" type="submit" :busy="busy" :disabled="!username || !password">
            <template #icon><LogIn class="button-icon" aria-hidden="true" /></template>
            {{ busy ? 'Входим...' : 'Войти' }}
          </SamsungButton>

          <SamsungButton
            v-if="account.enabled"
            variant="secondary"
            class="login-submit mt-3"
            type="button"
            @click="signInWithAccount"
          >
            <template #icon><KeyRound class="button-icon" aria-hidden="true" /></template>
            Войти через {{ account.name }}
          </SamsungButton>

          <p v-if="accountError" class="state-error mt-3">{{ accountError }}</p>

          <router-link v-if="registrationState.mode !== 'closed'" class="login-back-link" :to="{ name: 'register' }"
            >Создать аккаунт</router-link
          >
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
import InviteHero from '@/components/domain/InviteHero.vue';

import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { KeyRound, LogIn } from 'lucide-vue-next';
import { login, registrationState } from '@/stores/auth.js';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';

const router = useRouter();

const route = useRoute();
const needsTotp = ref(false);
const totpCode = ref('');
const inviteToken = ref(new URLSearchParams(window.location.search).get('invite') || '');
const username = ref('');
const password = ref('');
const error = ref('');
const busy = ref(false);
const year = computed(() => new Date().getFullYear());

const account = reactive({ enabled: false, name: 'WINGS Account' });

// Whatever went wrong happened on a redirect, so the reason arrives in the URL.
// Spelling it out beats leaving somebody staring at a login form that just
// bounced them for no stated reason.
const accountReasons = {
  expired: 'Вход просрочен, попробуйте ещё раз.',
  suspended: 'Аккаунт отключён.',
  registration_closed: 'Регистрация закрыта.',
  invite_required: 'Нужен инвайт.',
  username_taken: 'Такой логин уже занят в панели.',
  already_linked: 'Этот аккаунт уже привязан к другому админу.',
  link_failed: 'Не удалось привязать аккаунт.',
  exchange_failed: 'Сервис учёток не подтвердил вход.',
  login_failed: 'Вход не удался.',
};

const accountError = computed(() => {
  const code = route.query.account_error;
  if (!code) return '';
  return accountReasons[code] || `Вход через ${account.name} не удался.`;
});

onMounted(async () => {
  try {
    const res = await fetch('/api/oidc/status', { credentials: 'include' });
    if (res.ok) Object.assign(account, await res.json());
  } catch {
    // No account service is a normal state, not something to shout about.
  }
});

function signInWithAccount() {
  const back = route.query.redirect || '/admin/clients';
  // Код приглашения переносится и во вход: пригласить можно и того, у кого
  // аккаунт уже есть - тогда он получает пригласившего, а не второй аккаунт
  const params = new URLSearchParams({ return_to: back });
  const invite = inviteToken.value;
  if (invite) params.set('invite', invite);
  window.location.href = `/api/oidc/start?${params.toString()}`;
}

// Код второго фактора у WINGS Account проверяет провайдер, поэтому вход
// удерживается на его стороне, а у нас остаётся только квиток
const accountTicket = ref('');

// Одна форма на два входа. Сначала пробуем свой пароль, потом учётку: у
// половины админов пока только пароль, и заставлять их выбирать дверь глупо
async function onSubmit() {
  if (busy.value) return;
  busy.value = true;
  error.value = '';
  if (accountTicket.value) {
    await submitAccountFactor();
    return;
  }
  try {
    await login(username.value.trim().toLowerCase(), password.value, totpCode.value);
    const target = typeof route.query.redirect === 'string' ? route.query.redirect : '/admin/clients';
    // Возврат в приложение идёт мимо роутера: /app/link - серверный адрес,
    // который отдаёт редирект на схему приложения, а не страницу панели
    if (target.startsWith('/app/')) {
      window.location.assign(target);
      return;
    }
    router.push(target);
  } catch (err) {
    if (err.totpRequired) {
      needsTotp.value = true;
      error.value = totpCode.value ? 'Код не подошёл' : '';
      totpCode.value = '';
      return;
    }
    if (account.enabled && err.status === 401) {
      await submitAccountLogin();
      return;
    }
    error.value = err.message || 'Не удалось войти';
  } finally {
    busy.value = false;
  }
}

// Вход через учётку: форма наша, проверяет её сервис учёток, а обратно приходит
// адрес, по которому браузер возвращается уже с кодом
async function submitAccountLogin() {
  try {
    const res = await fetch('/api/account/login', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        login: username.value.trim(),
        password: password.value,
        return_to: typeof route.query.redirect === 'string' ? route.query.redirect : '',
        invite: inviteToken.value || '',
      }),
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Не удалось войти');
    if (data.second_factor) {
      accountTicket.value = data.ticket;
      needsTotp.value = true;
      totpCode.value = '';
      error.value = '';
      return;
    }
    window.location.assign(data.redirect);
  } catch (err) {
    error.value = err.message || 'Не удалось войти';
  } finally {
    busy.value = false;
  }
}

async function submitAccountFactor() {
  try {
    const res = await fetch('/api/account/factor', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ticket: accountTicket.value, code: totpCode.value }),
    });
    const data = await res.json();
    if (!res.ok) {
      // Квиток одноразовый: провайдер его уже съел, и вход начинается заново
      accountTicket.value = '';
      needsTotp.value = false;
      throw new Error(data.error || 'Код не подошёл');
    }
    window.location.assign(data.redirect);
  } catch (err) {
    error.value = err.message || 'Код не подошёл';
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
