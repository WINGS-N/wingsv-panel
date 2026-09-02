<template>
  <section class="surface-card">
    <div class="federation-live-head">
      <h2 class="section-title">Поддержать</h2>
      <span v-if="totalMicro" class="admin-pill is-info">{{ usdt(totalMicro) }} USDT от вас</span>
    </div>
    <p class="body-copy body-copy-wide">
      Два разных кошелька и два разных назначения. Переводы только в USDT в сети Solana.
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
  </section>

  <section class="surface-card mt-6">
    <h2 class="section-title">На трафик</h2>
    <p class="admin-muted mt-1">
      Идёт в общий котёл и раздаётся владельцам серверов за отданный трафик. Занос сюда поднимает ваш уровень доверия.
    </p>
    <p v-if="!wallets.traffic" class="state-hint mt-4">Кошелёк ещё не настроен.</p>
    <CopyableLink v-else :value="wallets.traffic" class="mt-4" />
  </section>

  <section class="surface-card mt-6">
    <h2 class="section-title">На разработку</h2>
    <p class="admin-muted mt-1">
      Идёт на приложение, панель и инфраструктуру. Донорам не раздаётся и на уровень доверия не влияет.
    </p>
    <CopyableLink :value="wallets.dev" class="mt-4" />
  </section>

  <section class="surface-card mt-6">
    <h2 class="section-title">Засчитать перевод</h2>
    <p class="admin-muted mt-1">
      Вставьте подпись транзакции - мы поднимем её из сети и проверим сами. Занос на трафик после этого начнёт греть
      доверие.
    </p>
    <div class="mt-4 flex flex-wrap items-end gap-3">
      <OneuiInput v-model="signature" label="Подпись транзакции" class="min-w-[280px] flex-1" />
      <OneuiRadioGroup v-model="kind" :options="kindOptions" variant="pill" />
      <SamsungButton :busy="claiming" @click="claim">
        <template #icon><Check class="button-icon" aria-hidden="true" /></template>
        Засчитать
      </SamsungButton>
    </div>
    <p v-if="claimError" class="state-error mt-2">{{ claimError }}</p>
    <p v-else-if="claimNote" class="state-hint mt-2">{{ claimNote }}</p>

    <div v-if="mine.length" class="fed-node-list mt-4">
      <div v-for="row in mine" :key="row.reference" class="fed-node-row">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-[15px]">{{ usdt(row.amount_micro) }} USDT</span>
            <span class="admin-pill" :class="row.kind === 'traffic' ? 'is-online' : 'is-info'">
              {{ row.kind === 'traffic' ? 'на трафик' : 'на разработку' }}
            </span>
          </div>
          <span class="mt-1 flex flex-wrap items-center gap-3 text-[13px] text-wings-muted">
            <span>{{ when(row.at_unix) }}</span>
            <span class="admin-mono truncate">{{ row.reference.slice(0, 16) }}</span>
          </span>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { onMounted, ref } from 'vue';
import { Check } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import OneuiRadioGroup from '@/components/controls/OneuiRadioGroup.vue';
import CopyableLink from '@/components/domain/CopyableLink.vue';
import { formatUsdt as usdt } from '@/utils/format';

const wallets = ref({ traffic: '', dev: '', mint: '' });
const mine = ref([]);
const totalMicro = ref(0);
const loadError = ref('');
const signature = ref('');
const kind = ref('traffic');
const claiming = ref(false);
const claimError = ref('');
const claimNote = ref('');

const kindOptions = [
  { value: 'traffic', label: 'На трафик' },
  { value: 'dev', label: 'На разработку' },
];

onMounted(load);

async function load() {
  try {
    const res = await fetch('/api/admin/donations', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    wallets.value = data.wallets || { traffic: '', dev: '', mint: '' };
    mine.value = data.mine || [];
    totalMicro.value = Number(data.total_micro || 0);
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  }
}

async function claim() {
  claiming.value = true;
  claimError.value = '';
  claimNote.value = '';
  try {
    const res = await fetch('/api/admin/donations/claim', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ signature: signature.value.trim(), kind: kind.value }),
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'не вышло засчитать');
    claimNote.value = `Засчитано ${usdt(data.amount_micro)} USDT`;
    signature.value = '';
    await load();
  } catch (err) {
    claimError.value = String(err.message || err);
  } finally {
    claiming.value = false;
  }
}

function when(unix) {
  if (!unix) return '';
  return new Date(Number(unix) * 1000).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' });
}
</script>
