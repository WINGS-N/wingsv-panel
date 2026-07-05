<template>
  <section class="form-section">
    <h3
      :class="['form-section-title', collapsible ? 'form-section-title-collapsible' : '']"
      :role="collapsible ? 'button' : undefined"
      @click="collapsible ? (open = !open) : null"
    >
      <ChevronDown v-if="collapsible" :class="['form-section-chevron', open ? 'is-open' : '']" aria-hidden="true" />
      <span>{{ title }}</span>
    </h3>
    <div v-show="!collapsible || open" class="form-section-body">
      <slot />
    </div>
  </section>
</template>

<script setup>
import { ref } from 'vue';
import { ChevronDown } from 'lucide-vue-next';

const props = defineProps({
  title: { type: String, default: '' },
  collapsible: { type: Boolean, default: false },
  defaultCollapsed: { type: Boolean, default: false },
});

const open = ref(!props.defaultCollapsed);
</script>

<style scoped>
.form-section-title-collapsible {
  cursor: pointer;
  user-select: none;
  display: flex;
  align-items: center;
  gap: 0.4em;
  margin-bottom: 0;
}
.form-section-title-collapsible + .form-section-body {
  margin-top: 0.75rem;
}
.form-section-chevron {
  width: 1.05em;
  height: 1.05em;
  flex-shrink: 0;
  transition: transform 0.2s ease;
  transform: rotate(-90deg);
}
.form-section-chevron.is-open {
  transform: rotate(0deg);
}
</style>
