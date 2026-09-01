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
          <router-link class="admin-nav-link" :to="{ name: 'cabinet-account' }" active-class="is-active">
            <UserCog class="admin-nav-icon" aria-hidden="true" />
            <span>Аккаунт</span>
          </router-link>
          <router-link v-if="hasPanel" class="admin-nav-link" :to="{ name: 'admin-clients' }">
            <SlidersHorizontal class="admin-nav-icon" aria-hidden="true" />
            <span>Панель</span>
          </router-link>
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
import { LogOut, SlidersHorizontal, Ticket, UserCog, UserPlus } from 'lucide-vue-next';
import { authState, logout, myAvatarUrl, refreshSession } from '@/stores/auth.js';
import SamsungButton from '@/components/layout/SamsungButton.vue';

const router = useRouter();
const busy = ref(false);
const admin = computed(() => authState.value.admin);
const hasPanel = computed(() => Boolean(admin.value?.panel_access));

onMounted(() => {
  if (!admin.value) refreshSession();
});

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
