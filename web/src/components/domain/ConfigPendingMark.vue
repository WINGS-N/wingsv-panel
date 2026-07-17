<template>
  <button
    v-if="state"
    type="button"
    class="pending-mark"
    :aria-label="'Ожидает применения. Текущее значение клиента: ' + state.text"
    @mouseenter="open = true"
    @mouseleave="open = false"
    @focus="open = true"
    @blur="open = false"
    @click.stop.prevent="open = !open"
  >
    <Clock class="pending-mark-icon" aria-hidden="true" />
    <span v-if="open" class="pending-mark-tip" role="tooltip">
      Ожидает применения<br />
      Текущее значение клиента: <b>{{ state.text }}</b>
    </span>
  </button>
</template>

<script setup>
import { computed, inject, ref } from 'vue';
import { Clock } from 'lucide-vue-next';

// Marks a field whose shown (desired) value has not reached the device yet, and
// tells the admin what the device still has. Rendered inside the field's label,
// so it costs one tag per field and leaves the row markup untouched.
const props = defineProps({
  path: { type: String, required: true },
});

// Provided by ConfigFormEditor; absent in contexts with nothing to compare
// against (the master config editor), where the mark simply never renders.
const lookup = inject('configPending', null);
const open = ref(false);

const state = computed(() => (lookup ? lookup(props.path) : null));
</script>

<style scoped>
.pending-mark {
  position: relative;
  display: inline-flex;
  align-items: center;
  padding: 0;
  margin-left: 0.35em;
  border: 0;
  background: none;
  color: var(--oneui-warning, #e8830c);
  cursor: pointer;
  vertical-align: middle;
}
.pending-mark-icon {
  width: 1em;
  height: 1em;
}
.pending-mark-tip {
  position: absolute;
  left: 50%;
  bottom: calc(100% + 0.4em);
  z-index: 20;
  transform: translateX(-50%);
  min-width: 12rem;
  max-width: 18rem;
  padding: 0.45rem 0.6rem;
  border-radius: 0.5rem;
  background: var(--oneui-tooltip-bg, rgba(28, 28, 30, 0.96));
  color: #fff;
  font-size: 0.78rem;
  font-weight: 400;
  line-height: 1.35;
  text-align: left;
  white-space: normal;
  word-break: break-word;
  pointer-events: none;
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.25);
}
</style>
