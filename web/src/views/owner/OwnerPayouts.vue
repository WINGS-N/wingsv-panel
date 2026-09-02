<template>
  <section class="surface-card">
    <div class="federation-live-head">
      <h2 class="section-title">Выплаты</h2>
      <span v-if="totalMicro" class="admin-pill is-info">{{ usdt(totalMicro) }} USDT всего</span>
    </div>
    <p class="body-copy body-copy-wide">
      Период закрывается корнем дерева: голова считает, сколько донор отдал трафика, и фиксирует начисления одной
      записью. Платим только за байты, которые подписали клиенты.
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <p v-else-if="!enabled" class="state-hint">Федерация выключена.</p>
    <p v-else-if="note" class="state-hint">{{ note }}</p>
  </section>

  <section v-if="enabled && !note" class="surface-card mt-6">
    <h2 class="section-title">Закрытые периоды</h2>
    <SamsungSectionLoader v-if="loading" />
    <p v-else-if="!epochs.length" class="state-hint mt-4">Ещё ни один период не закрыт.</p>
    <div v-else class="fed-node-list mt-4">
      <div v-for="epoch in epochs" :key="epoch.number" class="fed-node-row">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-[15px]">Эпоха {{ epoch.number }}</span>
            <span class="admin-pill" :class="epoch.tx_ref ? 'is-online' : 'is-info'">
              {{ epoch.tx_ref ? 'в цепочке' : 'посчитана' }}
            </span>
            <span class="admin-pill">{{ usdt(epoch.total_micro) }} USDT</span>
          </div>
          <span class="mt-1 flex flex-wrap items-center gap-3 text-[13px] text-wings-muted">
            <span>{{ when(epoch.start_unix) }} - {{ when(epoch.end_unix) }}</span>
            <span>{{ epoch.leaves }} получателей</span>
            <span class="admin-mono truncate">{{ epoch.root_hex.slice(0, 16) }}</span>
          </span>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';
import { formatUsdt as usdt } from '@/utils/format';

const enabled = ref(false);
const loading = ref(true);
const loadError = ref('');
const note = ref('');
const epochs = ref([]);
const totalMicro = ref(0);
let timer = null;

onMounted(() => {
  load();
  timer = setInterval(load, 60000);
});

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});

async function load() {
  try {
    const res = await fetch('/api/owner/federation/epochs', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    enabled.value = Boolean(data.enabled);
    note.value = data.error || '';
    epochs.value = data.epochs || [];
    totalMicro.value = Number(data.total_micro || 0);
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    loading.value = false;
  }
}

function when(unix) {
  if (!unix) return '';
  return new Date(Number(unix) * 1000).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short' });
}
</script>
