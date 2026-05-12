<template>
  <div class="mt-6 grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-[14px] max-[720px]:grid-cols-1">
    <div
      v-for="fact in facts"
      :key="`${fact.label}-${fact.value}`"
      class="min-w-0 overflow-hidden rounded-[26px] border border-[rgba(255,255,255,0.07)] bg-wings-chip px-5 py-[18px]"
    >
      <div class="mb-2 text-[13px] leading-[1.35] text-[rgba(245,247,255,0.58)]">{{ fact.label }}</div>
      <div class="text-[17px] font-semibold leading-[1.35] text-wings-text [overflow-wrap:anywhere] [word-break:break-word]">
        {{ fact.value }}
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  facts: {
    type: Array,
    default: () => [],
  },
});
</script>
