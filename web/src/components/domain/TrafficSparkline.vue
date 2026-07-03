<template>
  <svg class="sparkline" :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" role="img" :aria-label="ariaLabel">
    <polyline v-if="rxPoints" class="spark-rx" fill="none" :points="rxPoints" />
    <polyline v-if="txPoints" class="spark-tx" fill="none" :points="txPoints" />
    <text v-if="!series.length" class="spark-empty" :x="W / 2" :y="H / 2">нет данных</text>
  </svg>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  series: { type: Array, default: () => [] },
  ariaLabel: { type: String, default: 'График трафика' },
});

const W = 600;
const H = 120;

const scale = computed(() => {
  let max = 1;
  for (const p of props.series) {
    max = Math.max(max, Number(p.rx_bytes) || 0, Number(p.tx_bytes) || 0);
  }
  return max;
});

function build(key) {
  const pts = props.series;
  if (!pts.length) return '';
  const max = scale.value;
  const n = pts.length;
  return pts
    .map((p, i) => {
      const x = n === 1 ? W : (i / (n - 1)) * W;
      const y = H - ((Number(p[key]) || 0) / max) * (H - 4) - 2;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');
}

const rxPoints = computed(() => build('rx_bytes'));
const txPoints = computed(() => build('tx_bytes'));
</script>

<style scoped>
.sparkline {
  width: 100%;
  height: 120px;
  display: block;
}
.spark-rx {
  stroke: #1259d1;
  stroke-width: 2;
  vector-effect: non-scaling-stroke;
}
.spark-tx {
  stroke: rgba(252, 252, 252, 0.35);
  stroke-width: 2;
  vector-effect: non-scaling-stroke;
}
.spark-empty {
  fill: rgba(252, 252, 252, 0.4);
  font-size: 14px;
  text-anchor: middle;
  dominant-baseline: middle;
}
</style>
