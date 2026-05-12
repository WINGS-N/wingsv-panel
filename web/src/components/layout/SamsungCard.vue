<template>
  <section :class="['surface-card', $attrs.class]">
    <header
      v-if="kicker || title || $slots.title || $slots.actions"
      class="admin-card-header"
    >
      <div>
        <div
          v-if="kicker"
          class="text-[12px] font-bold uppercase tracking-[0.14em] text-wings-kicker"
        >
          {{ kicker }}
        </div>
        <slot name="title">
          <h2 v-if="title" class="admin-card-title">{{ title }}</h2>
        </slot>
        <p v-if="subtitle" class="body-copy">{{ subtitle }}</p>
        <slot name="subtitle" />
      </div>
      <div v-if="$slots.actions" class="flex items-center gap-2">
        <slot name="actions" />
      </div>
    </header>
    <slot />
  </section>
</template>

<script setup>
defineProps({
  kicker: { type: String, default: "" },
  title: { type: String, default: "" },
  subtitle: { type: String, default: "" },
});
defineOptions({ inheritAttrs: false });
</script>
