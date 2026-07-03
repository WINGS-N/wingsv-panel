<template>
  <div ref="wrap" class="fg-wrap" @pointermove="track">
    <template v-if="graph">
      <div class="fg-legend">
        <span>Клиенты</span>
        <span>Реле</span>
        <span>Назначения</span>
      </div>
      <svg class="fg-svg" :viewBox="`0 0 ${W} ${graph.height}`" role="img" aria-label="Граф потоков трафика">
        <path
          v-for="(l, i) in graph.links"
          :key="'lk' + i"
          class="fg-link"
          :class="{ 'fg-link-hot': hovered === i }"
          :d="l.d"
          :style="{ strokeWidth: 0 }"
          @pointerenter="hovered = i"
          @pointerleave="hovered = -1"
        />
        <g v-for="(a, i) in graph.arrows" :key="'ar' + i">
          <path class="fg-arrow" :d="a" />
        </g>

        <g v-for="node in graph.nodes" :key="node.id">
          <rect class="fg-node" :class="node.colClass" :x="node.x" :y="node.y" :width="NODEW" :height="node.h" rx="3" />
          <text
            v-if="node.h >= 10"
            class="fg-label"
            :class="'fg-label-' + node.col"
            :x="node.labelX"
            :y="node.y + node.h / 2"
            :text-anchor="node.anchor"
          >
            {{ node.label }}
          </text>
        </g>
      </svg>
    </template>
    <p v-else class="fg-empty">Нет активных соединений.</p>

    <div v-if="hovered >= 0 && graph" class="fg-tip" :style="tipStyle">
      <div class="fg-tip-path">{{ graph.links[hovered].label }}</div>
      <div class="fg-tip-bytes">{{ fmt(graph.links[hovered].bytes) }}</div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue';
import { formatBytes } from '@/utils/format.js';

const props = defineProps({
  flows: { type: Array, default: () => [] },
  nodeNames: { type: Object, default: () => ({}) },
  maxPerColumn: { type: Number, default: 8 },
});

const W = 720;
const NODEW = 11;
const PADY = 16;
const GAP = 9;
const CAP_H = 150;
const COLS = { c: 150, r: 355, d: 559 };

const fmt = (b) => formatBytes(Number(b) || 0);

const wrap = ref(null);
const hovered = ref(-1);
const pos = ref({ x: 0, y: 0 });

function track(e) {
  const el = wrap.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  pos.value = { x: e.clientX - rect.left, y: e.clientY - rect.top };
}

const tipStyle = computed(() => ({
  left: `${Math.min(pos.value.x + 12, 300)}px`,
  top: `${pos.value.y + 12}px`,
}));

function label(kind, key) {
  if (key === '__other__') return 'прочие';
  if (kind === 'r') return props.nodeNames[key] || (key.length > 10 ? key.slice(0, 8) : key);
  const s = key || '-';
  return s.length > 26 ? s.slice(0, 25) + '...' : s;
}

// fold keeps the top-N keys by bytes and collapses the rest into __other__ so the
// graph stays readable; link maps are rebuilt against the folded keys to keep the
// Sankey mass-conserving (a node's height equals the sum of its ribbons).
function fold(totals, limit) {
  const entries = [...totals.entries()].sort((a, b) => b[1] - a[1]);
  const keep = entries.slice(0, limit);
  const rest = entries.slice(limit);
  const map = new Map();
  const bytesOf = new Map();
  for (const [k, v] of keep) {
    map.set(k, k);
    bytesOf.set(k, v);
  }
  let restBytes = 0;
  for (const [k, v] of rest) {
    map.set(k, '__other__');
    restBytes += v;
  }
  const order = keep.map(([k]) => k);
  if (rest.length) {
    order.push('__other__');
    bytesOf.set('__other__', restBytes);
  }
  return { order, map, bytesOf };
}

const graph = computed(() => {
  const flows = props.flows.filter((f) => (Number(f.rx_bytes) || 0) + (Number(f.tx_bytes) || 0) > 0);
  if (!flows.length) return null;

  const cTot = new Map();
  const rTot = new Map();
  const dTot = new Map();
  const add = (m, k, v) => m.set(k, (m.get(k) || 0) + v);
  for (const f of flows) {
    const bytes = (Number(f.rx_bytes) || 0) + (Number(f.tx_bytes) || 0);
    add(cTot, f.client_ip || '-', bytes);
    add(rTot, f.node_id || '-', bytes);
    add(dTot, f.remote || '-', bytes);
  }
  const c = fold(cTot, props.maxPerColumn);
  const r = fold(rTot, props.maxPerColumn);
  const d = fold(dTot, props.maxPerColumn);

  const linkCR = new Map();
  const linkRD = new Map();
  for (const f of flows) {
    const bytes = (Number(f.rx_bytes) || 0) + (Number(f.tx_bytes) || 0);
    const ck = c.map.get(f.client_ip || '-');
    const rk = r.map.get(f.node_id || '-');
    const dk = d.map.get(f.remote || '-');
    add(linkCR, ck + '' + rk, bytes);
    add(linkRD, rk + '' + dk, bytes);
  }

  const total = [...cTot.values()].reduce((a, b) => a + b, 0) || 1;
  const maxCount = Math.max(c.order.length, r.order.length, d.order.length);
  const maxNodeBytes = Math.max(...[...c.bytesOf.values(), ...r.bytesOf.values(), ...d.bytesOf.values()], 1);
  const height = Math.max(180, PADY * 2 + maxCount * 30);
  const usableH = height - PADY * 2 - (maxCount - 1) * GAP;
  const scale = Math.min(usableH / total, CAP_H / maxNodeBytes);

  const nodes = [];
  const nodeIndex = new Map();
  function layout(col, colKey, order, bytesOf, x, labelX, anchor) {
    const colH = order.reduce((s, k) => s + Math.max(2, bytesOf.get(k) * scale), 0) + (order.length - 1) * GAP;
    let y = (height - colH) / 2;
    for (const k of order) {
      const h = Math.max(2, bytesOf.get(k) * scale);
      const node = {
        id: colKey + ':' + k,
        col: colKey,
        colClass: 'fg-node-' + colKey,
        x,
        y,
        h,
        labelX,
        anchor,
        label: label(colKey, k) + '  ' + fmt(bytesOf.get(k)),
        outY: y,
        inY: y,
      };
      nodes.push(node);
      nodeIndex.set(colKey + ':' + k, node);
      y += h + GAP;
    }
  }
  layout('c', 'c', c.order, c.bytesOf, COLS.c - NODEW, COLS.c - NODEW - 8, 'end');
  layout('r', 'r', r.order, r.bytesOf, COLS.r, COLS.r + NODEW / 2, 'middle');
  layout('d', 'd', d.order, d.bytesOf, COLS.d, COLS.d + NODEW + 8, 'start');

  const links = [];
  const arrows = [];
  function ribbon(srcNode, dstNode, bytes, srcLabel, dstLabel) {
    const w = Math.max(1, bytes * scale);
    const x1 = srcNode.x + NODEW;
    const x2 = dstNode.x;
    const sy = srcNode.outY;
    const ty = dstNode.inY;
    srcNode.outY += w;
    dstNode.inY += w;
    const xm = (x1 + x2) / 2;
    const d =
      `M${x1} ${sy} C${xm} ${sy} ${xm} ${ty} ${x2} ${ty}` +
      ` L${x2} ${ty + w} C${xm} ${ty + w} ${xm} ${sy + w} ${x1} ${sy + w} Z`;
    links.push({ d, bytes, label: srcLabel + ' → ' + dstLabel });
    const ac = ty + w / 2;
    const ax = x2 - 1;
    arrows.push(`M${ax - 7} ${ac - 4} L${ax} ${ac} L${ax - 7} ${ac + 4} Z`);
  }

  for (const ck of c.order) {
    for (const rk of r.order) {
      const bytes = linkCR.get(ck + '' + rk);
      if (bytes) ribbon(nodeIndex.get('c:' + ck), nodeIndex.get('r:' + rk), bytes, label('c', ck), label('r', rk));
    }
  }
  for (const rk of r.order) {
    for (const dk of d.order) {
      const bytes = linkRD.get(rk + '' + dk);
      if (bytes) ribbon(nodeIndex.get('r:' + rk), nodeIndex.get('d:' + dk), bytes, label('r', rk), label('d', dk));
    }
  }

  return { height, nodes, links, arrows };
});
</script>

<style scoped>
.fg-wrap {
  position: relative;
  width: 100%;
}
.fg-legend {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: rgba(252, 252, 252, 0.45);
  margin-bottom: 6px;
  padding: 0 4px;
}
.fg-svg {
  width: 100%;
  height: auto;
  display: block;
}
.fg-link {
  fill: rgba(75, 141, 255, 0.22);
  stroke: none;
  transition: fill 0.12s ease;
  cursor: pointer;
}
.fg-link-hot {
  fill: rgba(75, 141, 255, 0.55);
}
.fg-arrow {
  fill: rgba(75, 141, 255, 0.7);
}
.fg-node {
  fill: #4b8dff;
}
.fg-node-c {
  fill: #4b8dff;
}
.fg-node-r {
  fill: #b07cff;
}
.fg-node-d {
  fill: #5ecb9e;
}
.fg-label {
  fill: rgba(252, 252, 252, 0.82);
  font-size: 10px;
  dominant-baseline: middle;
  font-family: 'SamsungOne', system-ui, sans-serif;
}
.fg-empty {
  color: rgba(252, 252, 252, 0.55);
  font-size: 14px;
  margin: 12px 0;
}
.fg-tip {
  position: absolute;
  background: #1b1b1e;
  border: 1px solid rgba(252, 252, 252, 0.14);
  border-radius: 10px;
  padding: 8px 10px;
  font-size: 12px;
  color: #fbfbfb;
  pointer-events: none;
  white-space: nowrap;
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.4);
  z-index: 5;
}
.fg-tip-path {
  color: rgba(252, 252, 252, 0.7);
  margin-bottom: 3px;
}
.fg-tip-bytes {
  font-weight: 600;
}
</style>
