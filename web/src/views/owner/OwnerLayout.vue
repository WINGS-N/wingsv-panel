<template>
  <div class="admin-shell">
    <header class="admin-header">
      <div class="admin-brand-row">
        <div class="admin-brand">
          <span class="admin-brand-mark wordmark-inline">WINGS V</span>
          <span class="admin-brand-divider" aria-hidden="true">|</span>
          <span class="admin-brand-tag">Control Panel</span>
        </div>

        <div class="flex items-center gap-3">
          <span class="owner-role-pill" aria-label="Роль владельца">
            <span class="owner-role-pill-dot" aria-hidden="true"></span>
            <span>OWNER</span>
          </span>
          <router-link
            v-if="admin"
            :to="{ name: 'admin-account' }"
            class="admin-account-chip"
            aria-label="Текущий администратор"
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
        <h1 class="admin-page-label">Платформа</h1>
        <nav class="admin-nav" aria-label="Owner Console">
          <router-link class="admin-nav-link" :to="{ name: 'owner-overview' }" active-class="is-active">
            <LayoutDashboard class="admin-nav-icon" aria-hidden="true" />
            <span>Обзор</span>
          </router-link>
          <router-link class="admin-nav-link" :to="{ name: 'owner-admins' }" active-class="is-active">
            <UsersRound class="admin-nav-icon" aria-hidden="true" />
            <span>Администраторы</span>
          </router-link>
          <router-link class="admin-nav-link" :to="{ name: 'owner-clients' }" active-class="is-active">
            <Smartphone class="admin-nav-icon" aria-hidden="true" />
            <span>Все клиенты</span>
          </router-link>
          <router-link class="admin-nav-link" :to="{ name: 'owner-audit' }" active-class="is-active">
            <ClipboardList class="admin-nav-icon" aria-hidden="true" />
            <span>Аудит</span>
          </router-link>
          <router-link class="admin-nav-link" :to="{ name: 'admin-clients' }">
            <ArrowLeft class="admin-nav-icon" aria-hidden="true" />
            <span>В админ-панель</span>
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
import { computed, ref } from "vue";
import { useRouter } from "vue-router";
import { ArrowLeft, ClipboardList, LayoutDashboard, LogOut, Smartphone, UsersRound } from "lucide-vue-next";
import { authState, logout, myAvatarUrl } from "@/stores/auth.js";
import SamsungButton from "@/components/layout/SamsungButton.vue";

const router = useRouter();
const busy = ref(false);
const admin = computed(() => authState.value.admin);

const avatarInitials = computed(() => {
  const username = admin.value?.username || "";
  const parts = username.trim().split(/[\s._-]+/).filter(Boolean);
  if (parts.length === 0) return "·";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
});

async function onLogout() {
  busy.value = true;
  try {
    await logout();
  } finally {
    busy.value = false;
    router.push({ name: "login" });
  }
}
</script>
