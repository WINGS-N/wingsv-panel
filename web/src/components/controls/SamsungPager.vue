<template>
  <div v-if="pages > 1" class="pager">
    <SamsungButton variant="ghost" :disabled="page <= 1" @click="go(page - 1)">
      <template #icon><ChevronLeft class="button-icon" aria-hidden="true" /></template>
      Назад
    </SamsungButton>
    <span class="pager-state">
      {{ from }}-{{ to }} / {{ total }}
      <span class="pager-page">стр. {{ page }} / {{ pages }}</span>
    </span>
    <SamsungButton variant="ghost" :disabled="page >= pages" @click="go(page + 1)">
      <template #icon><ChevronRight class="button-icon" aria-hidden="true" /></template>
      Вперёд
    </SamsungButton>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { ChevronLeft, ChevronRight } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';

const props = defineProps({
  page: { type: Number, required: true },
  total: { type: Number, required: true },
  perPage: { type: Number, default: 25 },
});
const emit = defineEmits(['update:page']);

const pages = computed(() => Math.max(1, Math.ceil(props.total / props.perPage)));
const from = computed(() => (props.total === 0 ? 0 : (props.page - 1) * props.perPage + 1));
const to = computed(() => Math.min(props.total, props.page * props.perPage));

function go(next) {
  emit('update:page', Math.min(pages.value, Math.max(1, next)));
}
</script>
