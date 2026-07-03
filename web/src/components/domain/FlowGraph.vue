<template>
  <div ref="wrap" class="fg-wrap" @pointermove="track">
    <div class="fg-header">
      <div class="fg-stat">
        <span class="fg-stat-label">{{ mode === 'live' ? 'Сейчас' : 'Объём за период' }}</span>
        <span class="fg-stat-value">{{ headerThroughput }}</span>
      </div>
      <div class="fg-stat">
        <span class="fg-stat-label">Потоки</span>
        <span class="fg-stat-value">{{ flows.length }}</span>
      </div>
      <div class="fg-stat">
        <span class="fg-stat-label">Топ-потребитель</span>
        <span class="fg-stat-value">{{ topTalker }}</span>
      </div>
      <div class="fg-legend">
        <span v-for="p in legend" :key="p.key" class="fg-legend-item">
          <i class="fg-swatch" :style="{ background: p.color }" />{{ p.key }}
        </span>
      </div>
    </div>

    <svg
      v-if="graph"
      class="fg-svg"
      :width="W"
      :height="graph.height"
      :viewBox="`0 0 ${W} ${graph.height}`"
      role="img"
      aria-label="Граф потоков трафика"
    >
      <rect class="fg-bg" x="0" y="0" :width="W" :height="graph.height" @click="clearSelection" />
      <path
        v-for="l in graph.links"
        :key="l.key"
        class="fg-link"
        :class="{ 'fg-link-dim': dimmed(l), 'fg-link-hot': hoveredLink === l.key }"
        :d="l.d"
        :style="{ fill: l.color }"
        @pointerenter="hoveredLink = l.key"
        @pointerleave="hoveredLink = ''"
        @click.stop="pinLink(l)"
      />

      <template v-if="mode === 'live'">
        <path
          v-for="l in graph.links"
          :key="'flow' + l.key"
          class="fg-flow"
          :class="{ 'fg-link-dim': dimmed(l) }"
          :d="l.center"
          :style="{ stroke: l.color, animationDuration: l.flowDur }"
        />
      </template>

      <path v-for="a in graph.arrows" :key="a.key" class="fg-arrow" :class="{ 'fg-link-dim': dimmed(a) }" :d="a.d" />

      <g v-for="node in graph.nodes" :key="node.id">
        <rect
          class="fg-node"
          :class="[node.colClass, { 'fg-node-dim': nodeDimmed(node), 'fg-node-sel': selected === node.id }]"
          :x="node.x"
          :y="node.y"
          :width="NODEW"
          :height="node.h"
          rx="3"
          @pointerenter="hoveredNode = node.id"
          @pointerleave="hoveredNode = ''"
          @click.stop="toggleNode(node.id)"
        />
        <text
          v-if="node.h >= 9"
          class="fg-label"
          :class="{ 'fg-node-dim': nodeDimmed(node) }"
          :x="node.labelX"
          :y="node.y + node.h / 2"
          :text-anchor="node.anchor"
          @click.stop="toggleNode(node.id)"
        >
          {{ node.label }}
        </text>
      </g>
    </svg>
    <p v-else class="fg-empty">Нет данных о потоках за выбранный период.</p>

    <div v-if="tip" class="fg-tip" :style="tipStyle">
      <div class="fg-tip-path">{{ tip.label }}</div>
      <div class="fg-tip-bytes">всего {{ fmt(tip.rx + tip.tx) }} · ↓ {{ fmt(tip.rx) }} · ↑ {{ fmt(tip.tx) }}</div>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { formatBytes } from '@/utils/format.js';

const props = defineProps({
  flows: { type: Array, default: () => [] },
  nodeNames: { type: Object, default: () => ({}) },
  clientNames: { type: Object, default: () => ({}) },
  mode: { type: String, default: 'live' }, // live | historical
  maxPerColumn: { type: Number, default: 16 },
  // Recent average throughput (bytes/sec) from the parent, used for the live
  // header when the relay reports no per-flow rate.
  liveRate: { type: Number, default: 0 },
});

const NODEW = 11;
const PADY = 16;
const GAP = 9;
const MIN_H = 14; // every kept node is at least this tall so its label always shows
const TARGET_H = 440; // px the busiest column aims to fill; the SVG grows past it to fit

// Per-protocol colours; unknown protocols fall back to grey. Keys are lower-cased
// so the legend and links agree.
const PROTO_COLORS = {
  dtls: '#4b8dff',
  wg: '#5ecb9e',
  wireguard: '#5ecb9e',
  tcp: '#b07cff',
  udp: '#f2a65a',
  quic: '#e46a9b',
};
const OTHER_COLOR = 'rgba(200,200,210,0.5)';
const WELL_KNOWN_PORTS = { 443: 'HTTPS', 80: 'HTTP', 53: 'DNS', 22: 'SSH', 25: 'SMTP', 993: 'IMAPS', 3478: 'STUN' };

const fmt = (b) => formatBytes(Number(b) || 0);

const wrap = ref(null);
const W = ref(720);
let ro = null;
onMounted(() => {
  if (!wrap.value) return;
  ro = new ResizeObserver((entries) => {
    const w = entries[0]?.contentRect?.width || 0;
    if (w > 0) W.value = Math.max(360, Math.round(w));
  });
  ro.observe(wrap.value);
});
onBeforeUnmount(() => {
  if (ro) ro.disconnect();
});

const hoveredLink = ref('');
const pinnedLink = ref(null);
const hoveredNode = ref('');
const selected = ref('');
// Columns whose folded "прочие" bucket the operator expanded to full detail.
const expanded = ref({ c: false, r: false, d: false });
const pos = ref({ x: 0, y: 0 });

function track(e) {
  const el = wrap.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  pos.value = { x: e.clientX - rect.left, y: e.clientY - rect.top };
}

function protoColor(p) {
  return PROTO_COLORS[(p || '').toLowerCase()] || OTHER_COLOR;
}

function clientLabel(ip) {
  return props.clientNames[ip] || ip || '-';
}

function destLabel(remote) {
  const s = remote || '-';
  const idx = s.lastIndexOf(':');
  if (idx > 0) {
    const port = Number(s.slice(idx + 1));
    const svc = WELL_KNOWN_PORTS[port];
    if (svc) return `${s} ${svc}`;
  }
  return s;
}

function label(kind, key) {
  if (key === '__other__') return expanded.value[kind] ? 'прочие ▾' : 'прочие ▸';
  if (kind === 'r') return props.nodeNames[key] || (key.length > 10 ? key.slice(0, 8) : key);
  const s = kind === 'c' ? clientLabel(key) : destLabel(key);
  return s.length > 28 ? s.slice(0, 27) + '...' : s;
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

  const cols = { c: 0.2 * W.value, r: 0.5 * W.value, d: 0.8 * W.value };
  const bytesOf = (f) => (Number(f.rx_bytes) || 0) + (Number(f.tx_bytes) || 0);

  const cTot = new Map();
  const rTot = new Map();
  const dTot = new Map();
  const add = (m, k, v) => m.set(k, (m.get(k) || 0) + v);
  for (const f of flows) {
    const b = bytesOf(f);
    add(cTot, f.client_ip || '-', b);
    add(rTot, f.node_id || '-', b);
    add(dTot, f.remote || '-', b);
  }
  const limit = (col) => (expanded.value[col] ? Infinity : props.maxPerColumn);
  const c = fold(cTot, limit('c'));
  const r = fold(rTot, limit('r'));
  const d = fold(dTot, limit('d'));

  // Link accumulators carry rx/tx, a rate (live), and per-protocol byte tallies so
  // a folded link takes the colour of its dominant protocol.
  const linkCR = new Map();
  const linkRD = new Map();
  const acc = (m, key, f) => {
    let e = m.get(key);
    if (!e) {
      e = { rx: 0, tx: 0, rate: 0, protos: new Map() };
      m.set(key, e);
    }
    e.rx += Number(f.rx_bytes) || 0;
    e.tx += Number(f.tx_bytes) || 0;
    e.rate += (Number(f.rx_rate) || 0) + (Number(f.tx_rate) || 0);
    add(e.protos, (f.protocol || '').toLowerCase(), bytesOf(f));
  };
  for (const f of flows) {
    const ck = c.map.get(f.client_ip || '-');
    const rk = r.map.get(f.node_id || '-');
    const dk = d.map.get(f.remote || '-');
    acc(linkCR, ck + '|' + rk, f);
    acc(linkRD, rk + '|' + dk, f);
  }
  const dominantProto = (protos) => {
    let best = '';
    let bestB = -1;
    for (const [p, b] of protos) {
      if (b > bestB) {
        bestB = b;
        best = p;
      }
    }
    return best;
  };

  const total = [...cTot.values()].reduce((a, b) => a + b, 0) || 1;
  // Byte-to-pixel scale sizes ribbons; the busiest column aims for TARGET_H. Every
  // node then gets a MIN_H floor for a legible label, and the SVG height grows to
  // whatever the tallest column needs - so extra flows extend the graph downward
  // instead of being squished or clipped.
  const scale = TARGET_H / total;
  const nodeH = (b) => Math.max(MIN_H, b * scale);
  const colHeight = (order, bytesMap) =>
    order.reduce((s, k) => s + nodeH(bytesMap.get(k)), 0) + Math.max(0, order.length - 1) * GAP;
  const tallest = Math.max(colHeight(c.order, c.bytesOf), colHeight(r.order, r.bytesOf), colHeight(d.order, d.bytesOf));
  const height = Math.max(180, PADY * 2 + tallest);

  const nodes = [];
  const nodeIndex = new Map();
  function layout(colKey, order, bytesMap, x, labelX, anchor) {
    const colH = colHeight(order, bytesMap);
    let y = (height - colH) / 2;
    for (const k of order) {
      const h = nodeH(bytesMap.get(k));
      // Ribbons carry bytes*scale of the (possibly taller) node bar; centre them so
      // a floored node's ribbons sit in the middle rather than hugging the top.
      const ribbonH = bytesMap.get(k) * scale;
      const inset = Math.max(0, (h - ribbonH) / 2);
      const node = {
        id: colKey + ':' + k,
        col: colKey,
        colClass: 'fg-node-' + colKey,
        x,
        y,
        h,
        labelX,
        anchor,
        label: label(colKey, k) + '  ' + fmt(bytesMap.get(k)),
        outY: y + inset,
        inY: y + inset,
      };
      nodes.push(node);
      nodeIndex.set(node.id, node);
      y += h + GAP;
    }
  }
  layout('c', c.order, c.bytesOf, cols.c - NODEW, cols.c - NODEW - 8, 'end');
  layout('r', r.order, r.bytesOf, cols.r, cols.r + NODEW / 2, 'middle');
  layout('d', d.order, d.bytesOf, cols.d, cols.d + NODEW + 8, 'start');

  const links = [];
  const arrows = [];
  function ribbon(srcId, dstId, e, srcLabel, dstLabel) {
    const src = nodeIndex.get(srcId);
    const dst = nodeIndex.get(dstId);
    if (!src || !dst) return;
    const bytes = e.rx + e.tx;
    const w = Math.max(1, bytes * scale);
    const x1 = src.x + NODEW;
    const x2 = dst.x;
    const sy = src.outY;
    const ty = dst.inY;
    src.outY += w;
    dst.inY += w;
    const xm = (x1 + x2) / 2;
    const dPath =
      `M${x1} ${sy} C${xm} ${sy} ${xm} ${ty} ${x2} ${ty}` +
      ` L${x2} ${ty + w} C${xm} ${ty + w} ${xm} ${sy + w} ${x1} ${sy + w} Z`;
    const sc = sy + w / 2;
    const tc = ty + w / 2;
    const color = protoColor(dominantProto(e.protos));
    // Faster links animate quicker; clamp so nothing is dizzying or frozen.
    const flowDur = `${Math.max(0.6, 5 - Math.log10(1 + e.rate / 1024)).toFixed(2)}s`;
    links.push({
      key: srcId + '>' + dstId,
      d: dPath,
      center: `M${x1} ${sc} C${xm} ${sc} ${xm} ${tc} ${x2} ${tc}`,
      color,
      rx: e.rx,
      tx: e.tx,
      src: srcId,
      dst: dstId,
      flowDur,
      label: srcLabel + ' → ' + dstLabel,
    });
    arrows.push({
      key: srcId + '>' + dstId,
      d: `M${x2 - 8} ${tc - 4} L${x2 - 1} ${tc} L${x2 - 8} ${tc + 4} Z`,
      src: srcId,
      dst: dstId,
    });
  }

  for (const ck of c.order) {
    for (const rk of r.order) {
      const e = linkCR.get(ck + '|' + rk);
      if (e) ribbon('c:' + ck, 'r:' + rk, e, label('c', ck), label('r', rk));
    }
  }
  for (const rk of r.order) {
    for (const dk of d.order) {
      const e = linkRD.get(rk + '|' + dk);
      if (e) ribbon('r:' + rk, 'd:' + dk, e, label('r', rk), label('d', dk));
    }
  }

  return { height, nodes, links, arrows };
});

const legend = computed(() => {
  const seen = new Map();
  for (const f of props.flows) {
    const p = (f.protocol || '').toLowerCase();
    if (p && !seen.has(p)) seen.set(p, protoColor(p));
  }
  return [...seen.entries()].map(([key, color]) => ({ key, color }));
});

const headerThroughput = computed(() => {
  if (props.mode === 'live') {
    // Prefer live per-flow rates; the relay may report none, so fall back to the
    // parent's recent average so the figure is not stuck at 0 while traffic flows.
    let rate = 0;
    for (const f of props.flows) rate += (Number(f.rx_rate) || 0) + (Number(f.tx_rate) || 0);
    if (rate <= 0) rate = props.liveRate || 0;
    return fmt(rate) + '/s';
  }
  let bytes = 0;
  for (const f of props.flows) bytes += (Number(f.rx_bytes) || 0) + (Number(f.tx_bytes) || 0);
  return fmt(bytes);
});

const topTalker = computed(() => {
  const per = new Map();
  for (const f of props.flows) {
    const k = f.client_ip || '-';
    per.set(k, (per.get(k) || 0) + (Number(f.rx_bytes) || 0) + (Number(f.tx_bytes) || 0));
  }
  let best = '';
  let bestB = -1;
  for (const [k, v] of per) {
    if (v > bestB) {
      bestB = v;
      best = k;
    }
  }
  return best ? clientLabel(best) : '—';
});

// Selection/hover: a node click pins isolation; hovering a node or link lights up
// only the connected ribbons and endpoints, dimming the rest.
const activeNode = computed(() => selected.value || hoveredNode.value);
const activeLinkKey = computed(() => (pinnedLink.value ? pinnedLink.value.key : hoveredLink.value));

function dimmed(l) {
  if (activeLinkKey.value) return l.key !== activeLinkKey.value;
  if (activeNode.value) return l.src !== activeNode.value && l.dst !== activeNode.value;
  return false;
}
function nodeDimmed(node) {
  if (activeLinkKey.value) {
    const l = graph.value?.links.find((x) => x.key === activeLinkKey.value);
    return !l || (l.src !== node.id && l.dst !== node.id);
  }
  if (activeNode.value) {
    if (node.id === activeNode.value) return false;
    return !graph.value?.links.some(
      (l) => (l.src === activeNode.value && l.dst === node.id) || (l.dst === activeNode.value && l.src === node.id),
    );
  }
  return false;
}

// expandOtherFor toggles a column's "прочие" bucket to full detail when the given
// node id is that bucket. Returns true if it handled the id.
function expandOtherFor(id) {
  const sep = id.indexOf(':');
  if (id.slice(sep + 1) !== '__other__') return false;
  const col = id.slice(0, sep);
  expanded.value = { ...expanded.value, [col]: !expanded.value[col] };
  selected.value = '';
  pinnedLink.value = null;
  return true;
}

function toggleNode(id) {
  if (expandOtherFor(id)) return;
  selected.value = selected.value === id ? '' : id;
  pinnedLink.value = null;
}
function pinLink(l) {
  // A ribbon touching a folded "прочие" bucket expands that column instead of
  // pinning, so clicking the flow itself drills in.
  if (expandOtherFor(l.src) || expandOtherFor(l.dst)) return;
  pinnedLink.value = pinnedLink.value && pinnedLink.value.key === l.key ? null : l;
  selected.value = '';
}
function clearSelection() {
  selected.value = '';
  pinnedLink.value = null;
}

const tip = computed(() => {
  const key = activeLinkKey.value;
  if (!key) return null;
  return graph.value?.links.find((l) => l.key === key) || null;
});
const tipStyle = computed(() => ({
  left: `${Math.min(pos.value.x + 12, W.value - 180)}px`,
  top: `${pos.value.y + 12}px`,
}));
</script>

<style scoped>
.fg-wrap {
  position: relative;
  width: 100%;
}
.fg-header {
  display: flex;
  align-items: center;
  gap: 20px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.fg-stat {
  display: flex;
  flex-direction: column;
}
.fg-stat-label {
  font-size: 11px;
  color: rgba(252, 252, 252, 0.45);
}
.fg-stat-value {
  font-size: 15px;
  font-weight: 600;
  color: #fbfbfb;
}
.fg-legend {
  display: flex;
  gap: 12px;
  margin-left: auto;
  flex-wrap: wrap;
}
.fg-legend-item {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: rgba(252, 252, 252, 0.6);
}
.fg-swatch {
  width: 9px;
  height: 9px;
  border-radius: 3px;
  display: inline-block;
}
.fg-svg {
  width: 100%;
  height: auto;
  display: block;
}
.fg-bg {
  fill: transparent;
}
.fg-link {
  opacity: 0.85;
  stroke: none;
  transition:
    opacity 0.12s ease,
    fill 0.12s ease;
  cursor: pointer;
}
.fg-link-hot {
  opacity: 1;
}
.fg-link-dim {
  opacity: 0.12;
}
.fg-flow {
  fill: none;
  stroke-width: 2;
  stroke-dasharray: 2 10;
  opacity: 0.9;
  vector-effect: non-scaling-stroke;
  animation: fg-dash linear infinite;
  pointer-events: none;
}
@keyframes fg-dash {
  to {
    stroke-dashoffset: -24;
  }
}
.fg-arrow {
  fill: rgba(252, 252, 252, 0.55);
}
.fg-node {
  cursor: pointer;
  transition: opacity 0.12s ease;
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
.fg-node-dim {
  opacity: 0.22;
}
.fg-node-sel {
  stroke: #fbfbfb;
  stroke-width: 1.5;
}
.fg-label {
  fill: rgba(252, 252, 252, 0.82);
  font-size: 10px;
  dominant-baseline: middle;
  font-family: 'SamsungOne', system-ui, sans-serif;
  cursor: pointer;
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
