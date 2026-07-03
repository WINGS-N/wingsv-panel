<template>
  <div class="flow-chain-wrap">
    <svg
      v-if="shown.length"
      class="flow-chain"
      :viewBox="`0 0 ${W} ${height}`"
      role="img"
      aria-label="Цепочки активных соединений"
    >
      <g v-for="(f, i) in shown" :key="f.session_id + '/' + f.stream_id" :transform="`translate(0, ${i * ROW})`">
        <line class="fc-link" :x1="206" :y1="ROW / 2" :x2="250" :y2="ROW / 2" :stroke-width="linkW(f)" />
        <line class="fc-link" :x1="370" :y1="ROW / 2" :x2="430" :y2="ROW / 2" :stroke-width="linkW(f)" />

        <rect class="fc-box fc-client" x="6" :y="ROW / 2 - BOXH / 2" width="200" :height="BOXH" rx="8" />
        <text class="fc-label" x="106" :y="ROW / 2">{{ f.client_ip || '—' }}</text>

        <rect class="fc-box fc-stream" x="250" :y="ROW / 2 - BOXH / 2" width="120" :height="BOXH" rx="8" />
        <text class="fc-label" x="310" :y="ROW / 2">{{ f.protocol || 'stream' }} · {{ f.stream_id }}</text>

        <rect class="fc-box fc-remote" x="430" :y="ROW / 2 - BOXH / 2" width="244" :height="BOXH" rx="8" />
        <text class="fc-label" x="552" :y="ROW / 2">{{ f.remote || '—' }}</text>
      </g>
    </svg>
    <p v-else class="flow-empty">Нет активных соединений.</p>
  </div>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  flows: { type: Array, default: () => [] },
  max: { type: Number, default: 14 },
});

const W = 680;
const BOXH = 34;
const ROW = 52;

const shown = computed(() => props.flows.slice(0, props.max));
const height = computed(() => Math.max(ROW, shown.value.length * ROW));

function linkW(f) {
  const rate = (Number(f.rx_rate) || 0) + (Number(f.tx_rate) || 0);
  if (rate <= 0) return 1.5;
  // Log-scaled so a fast flow reads thicker without dwarfing the slow ones.
  return Math.min(10, 1.5 + Math.log10(1 + rate / 1024) * 2);
}
</script>

<style scoped>
.flow-chain {
  width: 100%;
  height: auto;
  display: block;
}
.fc-link {
  stroke: #1259d1;
  opacity: 0.6;
}
.fc-box {
  fill: #242427;
  stroke: rgba(252, 252, 252, 0.12);
  stroke-width: 1;
}
.fc-client {
  stroke: rgba(18, 89, 209, 0.55);
}
.fc-remote {
  stroke: rgba(252, 252, 252, 0.16);
}
.fc-label {
  fill: #fbfbfb;
  font-size: 11px;
  text-anchor: middle;
  dominant-baseline: middle;
  font-family: 'SamsungOne', system-ui, sans-serif;
}
.flow-empty {
  color: rgba(252, 252, 252, 0.55);
  font-size: 14px;
  margin: 12px 0;
}
</style>
