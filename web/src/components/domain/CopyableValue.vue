<template>
  <!-- Короткое значение с копированием. Отдельно от CopyableLink нарочно: тот
       под длинные ссылки и растит textarea, а адрес и код в такой простыне
       смотрятся уёбищно -->
  <div class="copy-value">
    <span class="copy-value-label">{{ label }}</span>
    <button type="button" class="copy-value-body" :title="'Скопировать: ' + value" @click="copy">
      <code class="copy-value-text">{{ value }}</code>
      <Check v-if="copied" :size="15" class="copy-value-icon" aria-hidden="true" />
      <Copy v-else :size="15" class="copy-value-icon" aria-hidden="true" />
    </button>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { Check, Copy } from 'lucide-vue-next';

defineProps({
  label: { type: String, default: '' },
  value: { type: String, required: true },
});

const copied = ref(false);
let timer = null;

async function copy(event) {
  const text = event.currentTarget.innerText.trim();
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    // Без разрешения на буфер остаётся выделение руками, и это не повод падать
    return;
  }
  copied.value = true;
  clearTimeout(timer);
  timer = setTimeout(() => (copied.value = false), 1500);
}
</script>
