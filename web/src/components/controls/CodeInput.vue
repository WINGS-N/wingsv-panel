<template>
  <!-- ============================================================
       ПОЛЕ КОДА - шесть ячеек вместо одной строки. Код всегда из
       шести цифр, и показывать это формой поля честнее, чем
       рассказывать словами.
       ============================================================ -->
  <div class="code-input" role="group" :aria-label="label">
    <input
      v-for="(cell, index) in cells"
      :key="index"
      ref="boxes"
      class="code-cell"
      type="text"
      inputmode="numeric"
      autocomplete="one-time-code"
      maxlength="1"
      :value="cell"
      :aria-label="`${label}, знак ${index + 1}`"
      @input="onInput(index, $event)"
      @keydown="onKey(index, $event)"
      @paste="onPaste(index, $event)"
      @focus="$event.target.select()"
    />
  </div>
</template>

<script setup>
import { computed, ref } from 'vue';

const props = defineProps({
  modelValue: { type: String, default: '' },
  length: { type: Number, default: 6 },
  label: { type: String, default: 'Код' },
});
const emit = defineEmits(['update:modelValue', 'complete']);

const boxes = ref([]);

const cells = computed(() => {
  const digits = digitsOf(props.modelValue);
  return Array.from({ length: props.length }, (_, i) => digits[i] || '');
});

function digitsOf(text) {
  return String(text || '').replace(/\D/g, '');
}

function push(digits) {
  const value = digits.slice(0, props.length);
  emit('update:modelValue', value);
  if (value.length === props.length) emit('complete', value);
  return value;
}

function focusCell(index) {
  const box = boxes.value[Math.min(Math.max(index, 0), props.length - 1)];
  if (box) box.focus();
}

function onInput(index, event) {
  const typed = digitsOf(event.target.value);
  const digits = digitsOf(props.modelValue).padEnd(props.length, ' ').split('');
  if (!typed) {
    // Стёрли знак: ячейка пустеет, а хвост остаётся на своих местах
    digits[index] = ' ';
    event.target.value = '';
    push(digits.join('').replace(/ /g, ''));
    return;
  }
  // Ввели несколько знаков разом - раскладываем от этой ячейки
  const merged = digits.join('').replace(/ /g, '').split('');
  typed.split('').forEach((digit, shift) => {
    if (index + shift < props.length) merged[index + shift] = digit;
  });
  const value = push(merged.join('').replace(/ /g, ''));
  event.target.value = typed[0];
  focusCell(index + typed.length);
  if (value.length === props.length) event.target.blur();
}

function onKey(index, event) {
  if (event.key === 'Backspace' && !event.target.value) {
    // Пустая ячейка отдаёт удаление соседу слева: иначе backspace ничего не
    // делает и кажется, что поле зависло
    event.preventDefault();
    const digits = digitsOf(props.modelValue).split('');
    digits.splice(index - 1, 1);
    push(digits.join(''));
    focusCell(index - 1);
    return;
  }
  if (event.key === 'ArrowLeft') {
    event.preventDefault();
    focusCell(index - 1);
  }
  if (event.key === 'ArrowRight') {
    event.preventDefault();
    focusCell(index + 1);
  }
}

function onPaste(index, event) {
  event.preventDefault();
  const pasted = digitsOf(event.clipboardData?.getData('text'));
  if (!pasted) return;
  // Вставили весь код целиком - раскладываем с первой ячейки, куда бы ни
  // вставляли: код копируют целиком, а не по знаку
  const whole = pasted.length >= props.length;
  const digits = whole ? pasted : digitsOf(props.modelValue).slice(0, index) + pasted;
  const value = push(digits);
  focusCell(value.length);
}
</script>
