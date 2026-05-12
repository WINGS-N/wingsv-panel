<template>
  <div class="admin-shell">
    <!-- ============================================================
         SAMSUNG ACCOUNT-STYLE HEADER
         Top row: WINGS V wordmark on the left, account chip + logout
         on the right (mirrors SAMSUNG wordmark + avatar on the
         reference). Title bar below: page label "Control Panel" plus
         underline-tab navigation (Клиенты / Аккаунт), exactly like
         "Учетная запись" + (Профиль / Безопасность / …).
         ============================================================ -->
    <header class="admin-header">
      <div class="admin-brand-row">
        <div class="admin-brand">
          <span class="admin-brand-mark wordmark-inline">WINGS V</span>
          <span class="admin-brand-divider" aria-hidden="true">|</span>
          <span class="admin-brand-tag">Control Panel</span>
        </div>

        <div class="flex items-center gap-3">
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
        <nav class="admin-nav" aria-label="Разделы панели">
          <router-link class="admin-nav-link" :to="{ name: 'admin-clients' }" active-class="is-active">
            <Users class="admin-nav-icon" aria-hidden="true" />
            <span>Клиенты</span>
          </router-link>
          <router-link class="admin-nav-link" :to="{ name: 'admin-master' }" active-class="is-active">
            <SlidersHorizontal class="admin-nav-icon" aria-hidden="true" />
            <span>Master</span>
          </router-link>
          <router-link class="admin-nav-link" :to="{ name: 'admin-account' }" active-class="is-active">
            <UserCog class="admin-nav-icon" aria-hidden="true" />
            <span>Аккаунт</span>
          </router-link>
          <router-link
            v-if="isOwner"
            class="admin-nav-link"
            :to="{ name: 'owner-overview' }"
            active-class="is-active"
          >
            <Crown class="admin-nav-icon" aria-hidden="true" />
            <span>Owner Console</span>
          </router-link>
        </nav>
      </div>
    </header>

    <main class="admin-main">
      <div v-if="admin && admin.must_change_password" class="admin-banner">
        <strong>Смените пароль администратора</strong>
        <p class="mt-1">
          Сейчас активен пароль по умолчанию.
          <router-link :to="{ name: 'admin-account' }">Перейти в «Аккаунт»</router-link>
        </p>
      </div>
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { Crown, LogOut, SlidersHorizontal, UserCog, Users } from "lucide-vue-next";
import { authState, isOwner, logout, myAvatarUrl, refreshSession } from "@/stores/auth.js";
import SamsungButton from "@/components/layout/SamsungButton.vue";

const router = useRouter();
const busy = ref(false);
const admin = computed(() => authState.value.admin);

// Two-letter monogram for the account chip (e.g. "NK" for "Nikita Kim").
const avatarInitials = computed(() => {
  const username = admin.value?.username || "";
  const parts = username.trim().split(/[\s._-]+/).filter(Boolean);
  if (parts.length === 0) return "·";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
});

onMounted(() => {
  if (!admin.value) {
    refreshSession();
  }
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
