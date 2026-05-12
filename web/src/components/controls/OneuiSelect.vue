<template>
  <div class="oneui-select-wrapper">
    <select class="oneui-select" :value="modelValue" :disabled="disabled" @change="onChange">
      <option v-for="opt in options" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
    </select>
    <ChevronDown class="oneui-select-chevron" aria-hidden="true" />
  </div>
</template>

<script setup>
import { ChevronDown } from "lucide-vue-next";
defineProps({
  modelValue: { type: [String, Number], default: "" },
  options: { type: Array, required: true },
  disabled: { type: Boolean, default: false },
});
const emit = defineEmits(["update:modelValue", "change"]);
function onChange(event) {
  emit("update:modelValue", event.target.value);
  emit("change", event.target.value);
}
</script>
