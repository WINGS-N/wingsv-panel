<template>
  <span v-if="applied !== null" :class="['config-applied', applied ? 'is-applied' : 'is-pending']">
    <Check v-if="applied" class="config-applied-icon" aria-hidden="true" />
    <Clock v-else class="config-applied-icon" aria-hidden="true" />
    <span>{{ applied ? 'Применено клиентом' : 'Ожидает применения' }}</span>
    <span v-if="detail" class="config-applied-detail">{{ detail }}</span>
  </span>
</template>

<script setup>
import { computed } from 'vue';
import { Check, Clock } from 'lucide-vue-next';
import { desiredApplied } from '@/utils/configDiff';

// Says whether the config the panel saved has reached the device. Null - meaning
// the badge renders nothing - when the device has never reported: "unknown" is
// the honest answer there, and a green tick would be a lie.
const props = defineProps({
  desired: { type: Object, default: null },
  reported: { type: Object, default: null },
  // Revision / timestamps line. Optional: the per-backend tabs show the state
  // alone, the config tab spells the details out.
  detail: { type: String, default: '' },
});

const applied = computed(() => {
  if (!props.desired || !props.reported) return null;
  return desiredApplied(props.desired, props.reported);
});
</script>
