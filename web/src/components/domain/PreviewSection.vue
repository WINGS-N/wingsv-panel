<template>
  <div :class="['preview-section', section.title === 'Попечитель' ? 'is-warning' : '']">
    <div class="preview-section-title">{{ section.title }}</div>
    <div v-if="section.note" class="preview-section-note">{{ section.note }}</div>
    <div v-if="section.facts && section.facts.length" class="preview-list">
      <div
        v-for="fact in section.facts"
        :key="`${section.title}-${fact.label}-${fact.value}`"
        class="preview-row"
      >
        <span class="preview-row-label">{{ fact.label }}</span>
        <span class="preview-row-value">{{ fact.value }}</span>
      </div>
    </div>
    <div v-if="section.items && section.items.length" class="preview-list preview-items">
      <div
        v-for="(item, index) in section.items"
        :key="`${section.title}-${index}-${item}`"
        class="preview-item"
      >
        <span class="preview-item-index">{{ index + 1 }}.</span>{{ item }}
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  section: {
    type: Object,
    required: true,
  },
});
</script>
