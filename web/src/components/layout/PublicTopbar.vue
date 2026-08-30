<template>
  <header class="samsung-topbar">
    <router-link class="samsung-topbar-brand" to="/">
      <span class="wordmark-inline">WINGS V</span>
      <template v-if="tag">
        <span class="samsung-topbar-divider">|</span>
        <span class="samsung-topbar-tag">{{ tag }}</span>
      </template>
    </router-link>

    <nav class="public-nav">
      <router-link class="public-nav-link" :to="{ name: 'landing' }" active-class="is-active">Приложение</router-link>
      <router-link class="public-nav-link" :to="{ name: 'federation-landing' }" active-class="is-active">
        Федерация
      </router-link>
      <router-link class="samsung-topbar-link" :to="{ name: 'login' }">Войти в панель</router-link>
    </nav>
  </header>
</template>

<script setup>
defineProps({
  // Приписка к вордмарку: "Federation" на странице федерации, пусто на главной
  tag: { type: String, default: '' },
});
</script>
