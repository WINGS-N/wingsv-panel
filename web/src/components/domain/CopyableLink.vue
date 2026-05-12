<template>
  <div class="copyable-link">
    <textarea
      ref="textareaRef"
      class="link-input copyable-link-text"
      :rows="rows"
      readonly
      :value="value"
    />
    <button type="button" class="button-secondary copyable-link-button" @click="copy">
      <SamsungLoader v-if="busy" />
      <span>{{ label }}</span>
    </button>
  </div>
</template>

<script setup>
import { ref } from "vue";
import SamsungLoader from "@/components/layout/SamsungLoader.vue";

const props = defineProps({
  value: { type: String, required: true },
  rows: { type: [String, Number], default: 3 },
});

const busy = ref(false);
const label = ref("Скопировать");
const textareaRef = ref(null);

async function copy() {
  busy.value = true;
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(props.value);
    } else if (textareaRef.value) {
      textareaRef.value.select();
      document.execCommand("copy");
    }
    label.value = "Скопировано";
    setTimeout(() => {
      label.value = "Скопировать";
    }, 1500);
  } catch (err) {
    label.value = "Ошибка";
    setTimeout(() => {
      label.value = "Скопировать";
    }, 1500);
  } finally {
    busy.value = false;
  }
}
</script>
