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

          <SamsungButton variant="secondary" class="login-submit mt-3" type="button" @click="startQR">
            <template #icon><QrCode class="button-icon" aria-hidden="true" /></template>
            Войти по QR-коду
          </SamsungButton>

          <div v-if="qr.url" class="qr-panel">
            <div class="qr-canvas-frame"><canvas ref="qrCanvas" width="220" height="220"></canvas></div>
            <p class="qr-hint">
              Наведите камеру телефона или сканер в приложении. На телефоне подтвердите вход - и эта страница откроется
              сама.
            </p>
            <p v-if="qr.state === 'expired'" class="state-error">Код просрочен, покажите новый.</p>
            <p v-else-if="qr.state === 'refused'" class="state-error">Вход отклонён.</p>
          </div>

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

import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { LogIn, QrCode } from 'lucide-vue-next';
import { authState, login, registrationState } from '@/stores/auth.js';
import { drawQR } from '@/utils/qr.js';
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

// Код второго фактора у WINGS Account проверяет провайдер, поэтому вход
// удерживается на его стороне, а у нас остаётся только квиток
const accountTicket = ref('');

// Вход по QR: код показываем здесь, а подтверждают с телефона, где человек уже
// вошёл. Пароль при этом не набирается вовсе
const qr = reactive({ url: '', code: '', state: 'idle' });
const qrCanvas = ref(null);
let qrTimer = 0;

onBeforeUnmount(stopQR);

async function startQR() {
  stopQR();
  try {
    const res = await fetch('/api/qr/start', { method: 'POST', credentials: 'include' });
    const data = await res.json();
    if (!res.ok) throw new Error(data.message || 'не вышло завести код');
    qr.code = data.code;
    qr.url = data.url;
    qr.state = 'pending';
    await paintQR(data.url);
    qrTimer = window.setInterval(pollQR, 1000);
  } catch (err) {
    error.value = String(err.message || err);
  }
}

function stopQR() {
  if (qrTimer) window.clearInterval(qrTimer);
  qrTimer = 0;
}

async function paintQR(link) {
  await nextTick();
  if (!qrCanvas.value) return;
  await drawQR(qrCanvas.value, link, 220);
}

async function pollQR() {
  try {
    const res = await fetch(`/api/qr/status?code=${encodeURIComponent(qr.code)}`, { credentials: 'include' });
    const data = await res.json();
    qr.state = data.state;
    if (data.state === 'approved') {
      stopQR();
      // Сессия уже стоит куки - дальше обычный путь после входа
      window.location.assign(typeof route.query.redirect === 'string' ? route.query.redirect : '/admin/clients');
      return;
    }
    if (data.state === 'expired' || data.state === 'refused') stopQR();
  } catch {
    // Сеть моргнула - следующий тик разберётся
  }
}

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
  // Сперва учётка: переехавшие ходят через неё, и панельный пароль им не нужен
  if (account.enabled && (await submitAccountLogin({ quiet: true }))) return;
  try {
    await login(username.value.trim().toLowerCase(), password.value, totpCode.value);
    // Панельный пароль подошёл, а учётки нет - человеку прямая дорога на переезд
    if (authState.value.admin?.account_link_needed) {
      router.push({ name: 'account-move' });
      return;
    }
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
    error.value = err.message || 'Не удалось войти';
  } finally {
    busy.value = false;
  }
}

// Вход через учётку: форма наша, проверяет её сервис учёток, а обратно приходит
// адрес, по которому браузер возвращается уже с кодом
async function submitAccountLogin({ quiet = false } = {}) {
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
    if (!res.ok) {
      // Молчим, пока есть вторая дверь: об ошибке скажет та, что не сработала
      if (quiet) return false;
      throw new Error(data.message || data.error || 'Не удалось войти');
    }
    if (data.second_factor) {
      accountTicket.value = data.ticket;
      needsTotp.value = true;
      totpCode.value = '';
      error.value = '';
      busy.value = false;
      return true;
    }
    window.location.assign(data.redirect);
    return true;
  } catch (err) {
    if (quiet) return false;
    error.value = err.message || 'Не удалось войти';
    busy.value = false;
    return false;
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
