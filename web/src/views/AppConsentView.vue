<template>
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
        <template v-if="!done">
          <h1 class="login-headline"><span>Разрешить доступ</span><span>к вашему аккаунту?</span></h1>

          <div class="consent-who">
            <span class="consent-mark" aria-hidden="true">WV</span>
            <span class="consent-who-text">
              <span class="consent-who-name">WINGS V</span>
              <span class="consent-who-meta">{{ device || 'приложение на этом устройстве' }}</span>
            </span>
          </div>

          <div class="consent-grants">
            <div class="consent-grant">
              <UserRound class="consent-grant-icon" aria-hidden="true" />
              <span class="consent-grant-text">
                <span class="consent-grant-title">Имя и фото</span>
                <span class="consent-grant-note">чтобы показать, кто вошёл</span>
              </span>
            </div>
            <div class="consent-grant">
              <Server class="consent-grant-icon" aria-hidden="true" />
              <span class="consent-grant-text">
                <span class="consent-grant-title">Ваши серверы и трафик</span>
                <span class="consent-grant-note">список доступа и расход за месяц</span>
              </span>
            </div>
            <div v-if="hasPanel" class="consent-grant">
              <ShieldCheck class="consent-grant-icon" aria-hidden="true" />
              <span class="consent-grant-text">
                <span class="consent-grant-title">Управление вашими клиентами</span>
                <span class="consent-grant-note">то же, что вы делаете в панели</span>
              </span>
            </div>
          </div>

          <p v-if="error" class="state-error mt-3">{{ error }}</p>

          <SamsungButton class="login-submit mt-4" :busy="busy" @click="allow">
            <template #icon><Check class="button-icon" aria-hidden="true" /></template>
            Разрешить
          </SamsungButton>
          <SamsungButton variant="secondary" class="login-submit mt-3" @click="cancel">Отмена</SamsungButton>
          <p class="consent-fineprint">Доступ отзывается в разделе аккаунта в любой момент.</p>
        </template>

        <template v-else>
          <div class="qr-done">
            <Check class="qr-done-mark" aria-hidden="true" />
            <p class="qr-done-title">Готово</p>
            <p class="qr-done-note">Возвращаемся в WINGS V. Если приложение не открылось само - нажмите кнопку.</p>
          </div>
          <SamsungButton class="login-submit mt-4" @click="openApp">Открыть WINGS V</SamsungButton>
        </template>
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
import { useRoute, useRouter } from 'vue-router';
import { Check, Server, ShieldCheck, UserRound } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import { authState, refreshSession } from '@/stores/auth.js';

const route = useRoute();
const router = useRouter();
const busy = ref(false);
const done = ref(false);
const error = ref('');
const redirect = ref('');
const year = computed(() => new Date().getFullYear());

const device = computed(() => String(route.query.device || '').slice(0, 60));
const hasPanel = computed(() => {
  const admin = authState.value.admin;
  return Boolean(admin && (admin.panel_access || admin.role === 'owner'));
});

onMounted(async () => {
  await refreshSession();
  // Спрашивать разрешение у того, кто ещё не вошёл, не о чем
  if (!authState.value.admin) {
    router.replace({ name: 'login', query: { redirect: route.fullPath } });
  }
});

async function allow() {
  busy.value = true;
  error.value = '';
  try {
    const res = await fetch('/api/app/consent', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ device_name: device.value }),
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.message || 'не вышло выдать доступ');
    redirect.value = data.redirect;
    done.value = true;
    window.location.assign(data.redirect);
  } catch (err) {
    error.value = String(err.message || err);
  } finally {
    busy.value = false;
  }
}

function openApp() {
  if (redirect.value) window.location.assign(redirect.value);
}

function cancel() {
  router.replace({ name: 'landing' });
}
</script>
