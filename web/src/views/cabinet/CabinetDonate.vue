<template>
  <section class="surface-card">
    <div class="federation-live-head">
      <h2 class="section-title">Поддержать</h2>
      <span v-if="totalMicro" class="admin-pill is-info">{{ usdt(totalMicro) }} USDT от вас</span>
    </div>
    <p class="body-copy body-copy-wide">
      Переводы в USDT, сеть Solana. Обязательно укажите свой код в поле memo - по нему платёж находит ваш аккаунт, и
      засчитывается он сам в течение минуты.
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>

    <div class="donate-grid mt-4">
      <div class="donate-card">
        <h3 class="donate-card-title">На трафик</h3>
        <p class="donate-card-note">В общий котёл, донорам за отданный трафик. Поднимает ваш уровень доверия.</p>
        <p v-if="!wallets.traffic" class="state-hint mt-3">Кошелёк ещё не настроен.</p>
        <CopyableValue v-else label="Кошелёк" :value="wallets.traffic" class="mt-3" />
      </div>

      <div class="donate-card">
        <h3 class="donate-card-title">На разработку</h3>
        <p class="donate-card-note">На приложение, панель и инфраструктуру. Донорам не раздаётся.</p>
        <CopyableValue label="Кошелёк" :value="wallets.dev" class="mt-3" />
      </div>

      <div class="donate-card">
        <h3 class="donate-card-title">Ваш код</h3>
        <p class="donate-card-note">Кладите его в memo перевода, иначе занос останется анонимным.</p>
        <CopyableValue label="memo" :value="memo" class="mt-3" />
      </div>

      <div class="donate-card">
        <h3 class="donate-card-title">Не нашлось?</h3>
        <p class="donate-card-note">Если перевод с кодом не появился, вставьте TXID - проверим руками.</p>
        <div class="mt-3 flex items-end gap-2">
          <OneuiInput v-model="signature" label="TXID" class="min-w-0 flex-1" />
          <SamsungButton :busy="claiming" @click="claim">Засчитать</SamsungButton>
        </div>
        <p v-if="claimError" class="state-error mt-2">{{ claimError }}</p>
        <p v-else-if="claimNote" class="state-hint mt-2">{{ claimNote }}</p>
      </div>
    </div>
  </section>

  <section v-if="mine.length" class="surface-card mt-6">
    <h2 class="section-title">Ваши заносы</h2>
    <div class="fed-node-list mt-2">
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
import SamsungButton from '@/components/layout/SamsungButton.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import CopyableValue from '@/components/domain/CopyableValue.vue';
import { formatUsdt as usdt } from '@/utils/format';

const wallets = ref({ traffic: '', dev: '', mint: '' });
const memo = ref('');
const mine = ref([]);
const totalMicro = ref(0);
const loadError = ref('');
const signature = ref('');
const claiming = ref(false);
const claimError = ref('');
const claimNote = ref('');

onMounted(load);

async function load() {
  try {
    const res = await fetch('/api/admin/donations', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    wallets.value = data.wallets || { traffic: '', dev: '', mint: '' };
    memo.value = data.memo || '';
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
      body: JSON.stringify({ signature: signature.value.trim(), kind: 'traffic' }),
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
