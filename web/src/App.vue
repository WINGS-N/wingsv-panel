<template>
  <router-view />
  <ConfirmDialog />
</template>

<script setup>
// All UI lives behind the router; see views/ for the landing/login/admin shells.
// ConfirmDialog is the app-wide confirmation modal driven by stores/confirm.js.
import ConfirmDialog from '@/components/layout/ConfirmDialog.vue';
</script>
