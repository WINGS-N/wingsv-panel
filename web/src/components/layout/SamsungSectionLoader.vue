<template>
  <div class="loading-layout">
    <div class="loading-shell">
      <div class="samsung-loader" aria-hidden="true">
        <div class="samsung-loader-dot samsung-loader-dot-top"></div>
        <div class="samsung-loader-dot samsung-loader-dot-right"></div>
        <div class="samsung-loader-dot samsung-loader-dot-bottom"></div>
        <div class="samsung-loader-dot samsung-loader-dot-left"></div>
      </div>
    </div>
    <p v-if="label" class="admin-muted mt-3">{{ label }}</p>
  </div>
</template>

<script setup>
defineProps({
  /** Optional helper text shown beneath the loader. */
  label: { type: String, default: "" },
});
</script>
