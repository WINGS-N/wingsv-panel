<template>
  <header class="samsung-topbar">
    <div class="public-bar-left">
      <router-link class="samsung-topbar-brand" to="/">
        <span class="wordmark-inline">WINGS V</span>
        <template v-if="tag">
          <span class="samsung-topbar-divider">|</span>
          <span class="samsung-topbar-tag">{{ tag }}</span>
        </template>
      </router-link>

      <nav class="public-nav">
        <router-link class="public-nav-link" :to="{ name: 'landing' }" active-class="is-active">Главная</router-link>
        <router-link class="public-nav-link" :to="{ name: 'federation-landing' }" active-class="is-active">
          Федерация
        </router-link>
      </nav>
    </div>

    <!-- Вошедшему предлагать "Войти" незачем: у него уже есть аккаунт, и вести
         его надо в панель, а не на форму входа -->
    <router-link v-if="admin" class="public-account" :to="{ name: 'admin-clients' }" :title="admin.username">
      <img :src="myAvatarUrl" alt="" class="public-avatar" />
      <span class="public-account-name">{{ admin.username }}</span>
    </router-link>
    <router-link v-else class="samsung-topbar-link" :to="{ name: 'login' }">Войти в панель</router-link>
  </header>
</template>

<script setup>
import { computed, onMounted } from 'vue';
import { authState, myAvatarUrl, refreshSession } from '@/stores/auth.js';

defineProps({
  // Приписка к вордмарку: "Federation" на странице федерации, пусто на главной
  tag: { type: String, default: '' },
});

const admin = computed(() => authState.value.admin);

onMounted(() => {
  if (!admin.value) refreshSession();
});
</script>
