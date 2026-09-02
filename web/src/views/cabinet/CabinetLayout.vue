<template>
  <div class="admin-shell">
    <header class="admin-header">
      <div class="admin-brand-row">
        <div class="admin-brand">
          <span class="admin-brand-mark wordmark-inline">WINGS V</span>
          <span class="admin-brand-divider" aria-hidden="true">|</span>
          <span class="admin-brand-tag">Federation</span>
        </div>

        <div class="flex items-center gap-3">
          <router-link
            v-if="admin"
            :to="{ name: 'cabinet-account' }"
            class="admin-account-chip"
            aria-label="Ваш аккаунт"
          >
            <span class="admin-account-chip-avatar" aria-hidden="true">
              <img :src="myAvatarUrl" alt="" />
            </span>
            <span class="admin-account-chip-name">{{ admin.username }}</span>
          </router-link>
          <SamsungButton variant="text" :busy="busy" @click="onLogout">
            <template #icon><LogOut class="button-icon" aria-hidden="true" /></template>
            Выйти
          </SamsungButton>
        </div>
      </div>

      <div class="admin-titlebar">
        <nav class="admin-nav" aria-label="Разделы кабинета">
          <router-link class="admin-nav-link" :to="{ name: 'cabinet-access' }" active-class="is-active">
            <Ticket class="admin-nav-icon" aria-hidden="true" />
            <span>Мой доступ</span>
          </router-link>
          <router-link class="admin-nav-link" :to="{ name: 'cabinet-invites' }" active-class="is-active">
            <UserPlus class="admin-nav-icon" aria-hidden="true" />
            <span>Приглашения</span>
          </router-link>
          <router-link class="admin-nav-link" :to="{ name: 'cabinet-donate' }" active-class="is-active">
            <HeartHandshake class="admin-nav-icon" aria-hidden="true" />
            <span>Поддержать</span>
          </router-link>
          <router-link class="admin-nav-link" :to="{ name: 'cabinet-account' }" active-class="is-active">
            <UserCog class="admin-nav-icon" aria-hidden="true" />
            <span>Аккаунт</span>
          </router-link>
          <!-- Панель открывается тем же нажатием: отдельного обряда для этого
               не нужно, если владелец не включил модерацию -->
          <a class="admin-nav-link" href="#" @click.prevent="openPanel">
            <SlidersHorizontal class="admin-nav-icon" aria-hidden="true" />
            <span>{{ panelBusy ? 'Открываем...' : 'Панель' }}</span>
          </a>
        </nav>
      </div>
    </header>

    <main class="admin-main">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { HeartHandshake, LogOut, SlidersHorizontal, Ticket, UserCog, UserPlus } from 'lucide-vue-next';
import { authState, logout, myAvatarUrl, refreshSession } from '@/stores/auth.js';
import SamsungButton from '@/components/layout/SamsungButton.vue';

const router = useRouter();
const busy = ref(false);
const panelBusy = ref(false);
const admin = computed(() => authState.value.admin);
const hasPanel = computed(() => Boolean(admin.value?.panel_access));

onMounted(() => {
  if (!admin.value) refreshSession();
});

// Нажали "Панель": у кого она есть - просто переходим, остальным открываем
async function openPanel() {
  if (hasPanel.value) {
    router.push({ name: 'admin-clients' });
    return;
  }
  panelBusy.value = true;
  try {
    const res = await fetch('/api/admin/me/panel-access', { method: 'POST', credentials: 'include' });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.message || 'Не удалось открыть панель');
    if (body.granted) {
      await refreshSession();
      router.push({ name: 'admin-clients' });
      return;
    }
    router.push({ name: 'cabinet-account' });
  } catch {
    router.push({ name: 'cabinet-account' });
  } finally {
    panelBusy.value = false;
  }
}

async function onLogout() {
  busy.value = true;
  try {
    await logout();
    router.push({ name: 'login' });
  } finally {
    busy.value = false;
  }
}
</script>
