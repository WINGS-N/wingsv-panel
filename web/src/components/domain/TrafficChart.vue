<template>
  <div ref="wrapEl" class="tc-wrap">
    <div class="tc-legend">
      <span class="tc-legend-item"><i class="tc-swatch tc-swatch-rx" /> Приём ↑</span>
      <span class="tc-legend-item"><i class="tc-swatch tc-swatch-tx" /> Передача ↓</span>
      <span v-if="span" class="tc-legend-span">{{ span }}</span>
    </div>
    <svg
      class="tc-svg"
      :width="W"
      :height="H"
      :viewBox="`0 0 ${W} ${H}`"
      role="img"
      :aria-label="ariaLabel"
      @pointermove="onMove"
      @pointerleave="hover = -1"
    >
      <line v-for="g in yTicks" :key="'yg' + g.v" class="tc-grid" :x1="padL" :x2="W - padR" :y1="g.y" :y2="g.y" />
      <text v-for="g in yTicks" :key="'yl' + g.v" class="tc-axis" :x="padL - 6" :y="g.y + 3" text-anchor="end">
        {{ g.label }}
      </text>

      <text v-for="t in xTicks" :key="'xl' + t.v" class="tc-axis" :x="t.x" :y="H - 6" text-anchor="middle">
        {{ t.label }}
      </text>

      <line class="tc-baseline" :x1="padL" :x2="W - padR" :y1="baseline" :y2="baseline" />
      <template v-if="series.length >= 2">
        <path class="tc-area tc-area-rx" :d="areaRx" />
        <path class="tc-area tc-area-tx" :d="areaTx" />
        <path class="tc-line tc-line-rx" :d="lineRx" fill="none" />
        <path class="tc-line tc-line-tx" :d="lineTx" fill="none" />
      </template>
      <template v-else-if="series.length === 1">
        <circle class="tc-dot tc-dot-rx" :cx="x(0)" :cy="rxY(series[0].rx_bytes)" r="3.5" />
        <circle class="tc-dot tc-dot-tx" :cx="x(0)" :cy="txY(series[0].tx_bytes)" r="3.5" />
      </template>
      <text v-else class="tc-empty" :x="W / 2" :y="baseline">нет данных за период</text>

      <g v-if="hoverPoint">
        <line class="tc-cursor" :x1="hoverX" :x2="hoverX" :y1="padT" :y2="H - padB" />
        <circle class="tc-dot tc-dot-rx" :cx="hoverX" :cy="rxY(hoverPoint.rx_bytes)" r="3" />
        <circle class="tc-dot tc-dot-tx" :cx="hoverX" :cy="txY(hoverPoint.tx_bytes)" r="3" />
      </g>
    </svg>

    <div v-if="hoverPoint" class="tc-tip" :style="tipStyle">
      <div class="tc-tip-time">{{ tipTime }}</div>
      <div class="tc-tip-row"><i class="tc-swatch tc-swatch-rx" />{{ fmt(hoverPoint.rx_bytes) }}</div>
      <div class="tc-tip-row"><i class="tc-swatch tc-swatch-tx" />{{ fmt(hoverPoint.tx_bytes) }}</div>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { formatBytes } from '@/utils/format.js';

const props = defineProps({
  series: { type: Array, default: () => [] },
  ariaLabel: { type: String, default: 'График трафика' },
});

// W tracks the real rendered width so the viewBox maps 1:1 to CSS pixels: no
// aspect-ratio stretch, so axis text and lines stay crisp instead of squashed.
const wrapEl = ref(null);
const W = ref(680);
const H = 240;
const padL = 56;
const padR = 12;
const padT = 12;
const padB = 24;

let ro = null;
onMounted(() => {
  if (!wrapEl.value) return;
  ro = new ResizeObserver((entries) => {
    const w = entries[0]?.contentRect?.width || 0;
    if (w > 0) W.value = Math.max(320, Math.round(w));
  });
  ro.observe(wrapEl.value);
});
onBeforeUnmount(() => {
  if (ro) ro.disconnect();
});

const fmt = (b) => formatBytes(Number(b) || 0);

const max = computed(() => {
  let m = 1;
  for (const p of props.series) {
    m = Math.max(m, Number(p.rx_bytes) || 0, Number(p.tx_bytes) || 0);
  }
  return niceCeil(m);
});

// niceCeil rounds a max up to a clean multiple of 10^n so the axis labels read
// well. Fine steps (not just 1/2/5) keep the scale tight against the peak, so low
// traffic fills the chart instead of hugging the baseline - the y-axis auto-zooms
// in on quiet periods and out on spikes because it always tracks the current peak.
function niceCeil(v) {
  if (v <= 1) return 1;
  const exp = Math.floor(Math.log10(v));
  const base = Math.pow(10, exp);
  const f = v / base;
  const step =
    f <= 1
      ? 1
      : f <= 1.2
        ? 1.2
        : f <= 1.5
          ? 1.5
          : f <= 2
            ? 2
            : f <= 2.5
              ? 2.5
              : f <= 3
                ? 3
                : f <= 4
                  ? 4
                  : f <= 5
                    ? 5
                    : f <= 6
                      ? 6
                      : f <= 8
                        ? 8
                        : 10;
  return step * base;
}

function xOf(i) {
  const n = props.series.length;
  if (n <= 1) return padL + (W.value - padL - padR) / 2;
  return padL + (i / (n - 1)) * (W.value - padL - padR);
}

// Bidirectional geometry: приём (rx) grows upward from the baseline, передача
// (tx) downward, mirrored around a shared zero line so direction reads at a glance.
const half = (H - padT - padB) / 2;
const baseline = padT + half;

function rxY(v) {
  return baseline - ((Number(v) || 0) / max.value) * half;
}
function txY(v) {
  return baseline + ((Number(v) || 0) / max.value) * half;
}

function linePath(yFn, key) {
  return props.series.map((p, i) => `${i === 0 ? 'M' : 'L'}${xOf(i).toFixed(1)} ${yFn(p[key]).toFixed(1)}`).join(' ');
}

function areaPath(yFn, key) {
  if (!props.series.length) return '';
  const top = props.series
    .map((p, i) => `${i === 0 ? 'M' : 'L'}${xOf(i).toFixed(1)} ${yFn(p[key]).toFixed(1)}`)
    .join(' ');
  const x0 = xOf(0).toFixed(1);
  const xn = xOf(props.series.length - 1).toFixed(1);
  return `${top} L${xn} ${baseline.toFixed(1)} L${x0} ${baseline.toFixed(1)} Z`;
}

const lineRx = computed(() => linePath(rxY, 'rx_bytes'));
const lineTx = computed(() => linePath(txY, 'tx_bytes'));
const areaRx = computed(() => areaPath(rxY, 'rx_bytes'));
const areaTx = computed(() => areaPath(txY, 'tx_bytes'));

const yTicks = computed(() => {
  const out = [];
  for (const f of [1, 0.5, 0, -0.5, -1]) {
    out.push({
      v: f,
      y: baseline - f * half,
      label: f === 0 ? '0' : fmt(Math.abs(f) * max.value),
    });
  }
  return out;
});

// The visible span decides the tick format: a sub-day window shows HH:MM, a
// multi-day window shows DD.MM (plus the hour only when the span is still short
// enough for intra-day detail to matter), so the axis actually reads differently
// across the 24h / 7d / month ranges.
const spanSeconds = computed(() => {
  const n = props.series.length;
  if (n < 2) return 0;
  return (Number(props.series[n - 1].ts) || 0) - (Number(props.series[0].ts) || 0);
});

const xTicks = computed(() => {
  const n = props.series.length;
  if (!n) return [];
  const want = Math.min(6, n);
  const out = [];
  for (let i = 0; i < want; i++) {
    const idx = want === 1 ? 0 : Math.round((i / (want - 1)) * (n - 1));
    out.push({ v: idx, x: xOf(idx), label: tickLabel(props.series[idx].ts) });
  }
  return out;
});

function pad2(v) {
  return String(v).padStart(2, '0');
}

function hhmm(ts) {
  const d = new Date((Number(ts) || 0) * 1000);
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

function ddmm(ts) {
  const d = new Date((Number(ts) || 0) * 1000);
  return `${pad2(d.getDate())}.${pad2(d.getMonth() + 1)}`;
}

// tickLabel adapts to the whole-chart span (not per-tick), so all axis labels
// share one format.
function tickLabel(ts) {
  const day = 86400;
  if (spanSeconds.value > 3 * day) return ddmm(ts);
  if (spanSeconds.value > day) return `${ddmm(ts)} ${hhmm(ts)}`;
  return hhmm(ts);
}

const span = computed(() => {
  const n = props.series.length;
  if (n < 2) return '';
  return `${tickLabel(props.series[0].ts)} - ${tickLabel(props.series[n - 1].ts)}`;
});

const hover = ref(-1);

// A stale hover index would point past a shorter series after a range/sample
// switch and crash the tooltip accessors, so drop it whenever the data changes.
watch(
  () => props.series,
  () => {
    hover.value = -1;
  },
);

function onMove(e) {
  const n = props.series.length;
  if (!n) return;
  const rect = e.currentTarget.getBoundingClientRect();
  const rel = ((e.clientX - rect.left) / rect.width) * W.value;
  const frac = (rel - padL) / (W.value - padL - padR);
  let idx = Math.round(frac * (n - 1));
  idx = Math.max(0, Math.min(n - 1, idx));
  hover.value = idx;
}

const hoverPoint = computed(() =>
  hover.value >= 0 && hover.value < props.series.length ? props.series[hover.value] : null,
);
const hoverX = computed(() => (hoverPoint.value ? xOf(hover.value) : 0));
const tipTime = computed(() => (hoverPoint.value ? tickLabel(hoverPoint.value.ts) : ''));
const tipStyle = computed(() => {
  const leftPct = (hoverX.value / W.value) * 100;
  const side = leftPct > 60 ? 'right' : 'left';
  return side === 'right'
    ? { right: `${(100 - leftPct).toFixed(1)}%`, left: 'auto' }
    : { left: `${leftPct.toFixed(1)}%`, right: 'auto' };
});
</script>

<style scoped>
.tc-wrap {
  position: relative;
  width: 100%;
}
.tc-legend {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 8px;
  font-size: 13px;
  color: rgba(252, 252, 252, 0.7);
}
.tc-legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.tc-legend-span {
  margin-left: auto;
  color: rgba(252, 252, 252, 0.45);
}
.tc-swatch {
  width: 10px;
  height: 10px;
  border-radius: 3px;
  display: inline-block;
}
.tc-swatch-rx {
  background: #4b8dff;
}
.tc-swatch-tx {
  background: #b07cff;
}
.tc-svg {
  width: 100%;
  height: 240px;
  display: block;
}
.tc-grid {
  stroke: rgba(252, 252, 252, 0.08);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}
.tc-baseline {
  stroke: rgba(252, 252, 252, 0.25);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}
.tc-axis {
  fill: rgba(252, 252, 252, 0.45);
  font-size: 10px;
  font-family: 'SamsungOne', system-ui, sans-serif;
}
.tc-line {
  stroke-width: 2;
  vector-effect: non-scaling-stroke;
}
.tc-line-rx {
  stroke: #4b8dff;
}
.tc-line-tx {
  stroke: #b07cff;
}
.tc-area-rx {
  fill: rgba(75, 141, 255, 0.16);
}
.tc-area-tx {
  fill: rgba(176, 124, 255, 0.12);
}
.tc-cursor {
  stroke: rgba(252, 252, 252, 0.3);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}
.tc-dot-rx {
  fill: #4b8dff;
}
.tc-dot-tx {
  fill: #b07cff;
}
.tc-empty {
  fill: rgba(252, 252, 252, 0.4);
  font-size: 14px;
  text-anchor: middle;
  dominant-baseline: middle;
}
.tc-tip {
  position: absolute;
  top: 34px;
  transform: translateX(8px);
  background: #1b1b1e;
  border: 1px solid rgba(252, 252, 252, 0.14);
  border-radius: 10px;
  padding: 8px 10px;
  font-size: 12px;
  color: #fbfbfb;
  pointer-events: none;
  white-space: nowrap;
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.4);
}
.tc-tip-time {
  color: rgba(252, 252, 252, 0.55);
  margin-bottom: 4px;
}
.tc-tip-row {
  display: flex;
  align-items: center;
  gap: 6px;
}
</style>
