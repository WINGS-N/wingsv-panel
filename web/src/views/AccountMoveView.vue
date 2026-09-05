<template>
  <!-- ============================================================
       ПЕРЕЕЗД НА ОБЩИЙ ВХОД - тот же самсунговский диалог, что и на
       странице входа: человек попадает сюда сразу после пароля, и
       менять ему обстановку посреди входа незачем.
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
        <h1 class="login-headline">
          <span>Один аккаунт.</span>
          <span>На все сервисы.</span>
        </h1>
        <p class="login-sub">Панель переезжает на {{ accountName }}. Заведите его один раз - и он откроет всё.</p>

        <form class="login-form" @submit.prevent="onSubmit">
          <div class="input-field">
            <OneuiInput :model-value="username" label="Логин" disabled />
          </div>

          <div class="input-field">
            <OneuiInput v-model.trim="email" label="Почта" type="email" autocomplete="email" />
          </div>

          <div class="input-field">
            <OneuiInput v-model="password" label="Новый пароль" type="password" autocomplete="new-password" />
          </div>

          <div class="input-field">
            <OneuiInput v-model="confirm" label="Ещё раз" type="password" autocomplete="new-password" />
          </div>

          <p v-if="error" class="state-error">{{ error }}</p>
          <p v-if="hint" class="state-hint">{{ hint }}</p>

          <SamsungButton class="login-submit" type="submit" :busy="busy" :disabled="!canSubmit">
            <template #icon><KeyRound class="button-icon" aria-hidden="true" /></template>
            {{ busy ? 'Заводим...' : `Завести ${accountName}` }}
          </SamsungButton>

          <p class="login-sub mt-3">
            Логин остаётся прежним, пароль от панели после переезда больше не нужен. Второй фактор и ключи входа
            настраиваются потом, в разделе аккаунта.
          </p>

          <button class="login-back-link" type="button" @click="signOut">Выйти</button>
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
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { KeyRound } from 'lucide-vue-next';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import { authState, logout, refreshSession } from '@/stores/auth.js';

const router = useRouter();
const email = ref('');
const password = ref('');
const confirm = ref('');
const error = ref('');
const hint = ref('');
const busy = ref(false);
const year = computed(() => new Date().getFullYear());

const username = computed(() => authState.value.admin?.username || '');
const accountName = computed(() => authState.value.admin?.account_name || 'WINGS Account');

const canSubmit = computed(
  () => email.value.includes('@') && password.value.length >= 8 && password.value === confirm.value,
);

onMounted(async () => {
  await refreshSession();
  // Переехавшему тут делать нечего: он попал сюда по старой ссылке
  if (!authState.value.admin?.account_link_needed) router.replace('/admin/clients');
});

async function onSubmit() {
  if (busy.value || !canSubmit.value) return;
  busy.value = true;
  error.value = '';
  hint.value = '';
  try {
    const res = await fetch('/api/account/enroll', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: email.value, password: password.value }),
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.message || 'не вышло завести учётку');
    hint.value = 'Готово, входим...';
    await refreshSession();
    router.replace('/admin/clients');
  } catch (err) {
    error.value = String(err.message || err);
  } finally {
    busy.value = false;
  }
}

async function signOut() {
  await logout();
  router.replace('/login');
}
</script>
