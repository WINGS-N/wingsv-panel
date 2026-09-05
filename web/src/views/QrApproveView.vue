<template>
  <!-- ============================================================
       ПОДТВЕРЖДЕНИЕ ВХОДА - открывается на телефоне: приложением по
       своей ссылке или системной камерой в браузере. Тот же диалог,
       что и на входе, потому что это и есть вход, только чужой.
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
        <h1 class="login-headline"><span>Впустить</span><span>эту машину?</span></h1>
        <p class="login-sub">Кто-то открыл панель и просит войти вашей учёткой</p>

        <div v-if="loading" class="mt-6 flex justify-center"><SamsungLoader /></div>

        <template v-else-if="asked">
          <div class="qr-facts">
            <div class="qr-fact">
              <span class="qr-fact-label">Адрес</span>
              <span class="qr-fact-value admin-mono">{{ asked.from_ip || 'неизвестен' }}</span>
            </div>
            <div class="qr-fact">
              <span class="qr-fact-label">Браузер</span>
              <span class="qr-fact-value">{{ shortAgent }}</span>
            </div>
            <div class="qr-fact">
              <span class="qr-fact-label">Запрошен</span>
              <span class="qr-fact-value">{{ askedAt }}</span>
            </div>
          </div>

          <p class="state-hint mt-4">Если это не вы - просто закройте страницу. Без подтверждения никого не пустят.</p>
          <p v-if="error" class="state-error mt-3">{{ error }}</p>

          <SamsungButton class="login-submit mt-4" :busy="busy" :disabled="done" @click="approve">
            <template #icon><Check class="button-icon" aria-hidden="true" /></template>
            {{ done ? 'Впущено' : 'Впустить' }}
          </SamsungButton>
        </template>

        <p v-else class="state-error mt-4">{{ error || 'Код просрочен. Обновите страницу входа и покажите новый.' }}</p>
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
import { Check } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungLoader from '@/components/layout/SamsungLoader.vue';
import { authState, refreshSession } from '@/stores/auth.js';

const route = useRoute();
const router = useRouter();
const asked = ref(null);
const loading = ref(true);
const busy = ref(false);
const done = ref(false);
const error = ref('');
const year = computed(() => new Date().getFullYear());

// Браузер целиком читать невозможно, а первое слово говорит достаточно
const shortAgent = computed(() => {
  const ua = asked.value?.user_agent || '';
  const known = ['Firefox', 'Chrome', 'Safari', 'Edg', 'YaBrowser'];
  const hit = known.find((name) => ua.includes(name));
  return hit === 'Edg' ? 'Edge' : hit || 'неизвестен';
});

const askedAt = computed(() => {
  const at = asked.value?.asked_at;
  if (!at) return '';
  return new Date(at).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' });
});

onMounted(async () => {
  await refreshSession();
  // Подтверждает только вошедший: сначала вход, потом обратно сюда
  if (!authState.value.admin) {
    router.replace({ name: 'login', query: { redirect: route.fullPath } });
    return;
  }
  try {
    const res = await fetch(`/api/qr/pending?code=${encodeURIComponent(route.params.code)}`, {
      credentials: 'include',
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.message || 'код не найден');
    asked.value = data;
  } catch (err) {
    error.value = String(err.message || err);
  } finally {
    loading.value = false;
  }
});

async function approve() {
  busy.value = true;
  error.value = '';
  try {
    const res = await fetch('/api/qr/approve', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code: route.params.code }),
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.message || 'не вышло впустить');
    done.value = true;
  } catch (err) {
    error.value = String(err.message || err);
  } finally {
    busy.value = false;
  }
}
</script>
