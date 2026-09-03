<template>
  <section class="surface-card">
    <div class="federation-live-head">
      <h2 class="section-title">Купленные подписки</h2>
      <span class="admin-pill" :class="enabled ? 'is-online' : 'is-offline'">
        {{ enabled ? 'раздаются' : 'выключены' }}
      </span>
    </div>
    <p class="body-copy body-copy-wide">
      Оплаченные подписки идут людям тем же списком, что и наши серверы. За чужим сервером наш агент не стоит: ни
      доменов, ни отпечатков, ни сверки объёма оттуда не видно, поэтому они достаются только тем, у кого доверие не ниже
      {{ minConfidence }}.
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <p v-else-if="note" class="state-hint">{{ note }}</p>

    <div class="actions-row mt-4">
      <SamsungButton :busy="busy" @click="toggleAll">
        <template #icon>
          <Power class="button-icon" aria-hidden="true" />
        </template>
        {{ enabled ? 'Выключить раздачу' : 'Включить раздачу' }}
      </SamsungButton>
    </div>
  </section>

  <section class="surface-card mt-6">
    <h2 class="section-title">Добавить</h2>
    <p class="admin-muted mt-1">
      Вендор попадёт в название сервера у человека. Устройство - то, чем мы представляемся продавцу: оно всегда одно и
      то же, иначе на той стороне это выглядит как толпа новых железок.
    </p>
    <div class="form-grid mt-4">
      <OneuiInput v-model="draft.vendor" label="Вендор" placeholder="как назвать в списке" />
      <OneuiInput v-model="draft.url" label="Ссылка на подписку" />
      <OneuiInput v-model="draft.device_id" label="Устройство (memo продавца)" />
      <OneuiInput v-model.number="draft.max_clients" label="Людей одновременно" type="number" :min="0" :max="1000" />
    </div>
    <p v-if="formError" class="state-error mt-2">{{ formError }}</p>
    <div class="actions-row mt-4">
      <SamsungButton :busy="busy" @click="save">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        Добавить
      </SamsungButton>
    </div>
  </section>

  <section v-if="sources.length" class="surface-card mt-6">
    <h2 class="section-title">Источники</h2>
    <div class="fed-cards">
      <div v-for="source in sources" :key="source.id" class="fed-card">
        <div class="fed-card-head">
          <span class="fed-card-name">{{ source.vendor || source.id }}</span>
          <span class="admin-pill shrink-0" :class="source.enabled ? 'is-online' : 'is-offline'">
            {{ source.enabled ? 'включён' : 'выключен' }}
          </span>
        </div>
        <div class="fed-card-facts">
          <div class="fed-card-fact">
            <span class="fed-card-fact-label">Серверов в подписке</span>
            <span class="fed-card-fact-value">{{ source.links }}</span>
          </div>
          <div class="fed-card-fact">
            <span class="fed-card-fact-label">Людей одновременно</span>
            <span class="fed-card-fact-value">{{ source.max_clients || 'без потолка' }}</span>
          </div>
          <div class="fed-card-fact">
            <span class="fed-card-fact-label">Читали</span>
            <span class="fed-card-fact-value">{{ when(source.fetched_unix) || '-' }}</span>
          </div>
          <div class="fed-card-fact">
            <span class="fed-card-fact-label">Устройство</span>
            <span class="fed-card-fact-value admin-mono">{{ source.device_id || '-' }}</span>
          </div>
        </div>
        <p v-if="source.last_error" class="state-error text-[13px]">{{ source.last_error }}</p>
        <div class="fed-node-actions">
          <SamsungButton variant="ghost" :busy="busy" @click="toggle(source)">
            {{ source.enabled ? 'Выключить' : 'Включить' }}
          </SamsungButton>
          <SamsungButton variant="ghost" :busy="busy" @click="remove(source)">
            <template #icon><Trash2 class="button-icon" aria-hidden="true" /></template>
            Убрать
          </SamsungButton>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue';
import { Plus, Power, Trash2 } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';

const enabled = ref(false);
const sources = ref([]);
const minConfidence = ref(95);
const loadError = ref('');
const note = ref('');
const formError = ref('');
const busy = ref(false);
const draft = reactive({ vendor: '', url: '', device_id: '', max_clients: 0 });

onMounted(load);

async function load() {
  try {
    const res = await fetch('/api/owner/federation/upstreams', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    apply(await res.json());
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  }
}

function apply(data) {
  enabled.value = Boolean(data.enabled);
  sources.value = data.sources || [];
  minConfidence.value = Number(data.min_confidence || 95);
  note.value = data.error || '';
}

async function send(body) {
  busy.value = true;
  formError.value = '';
  try {
    const res = await fetch('/api/owner/federation/upstreams', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(await res.text());
    apply(await res.json());
  } catch (err) {
    formError.value = String(err.message || err);
  } finally {
    busy.value = false;
  }
}

function toggleAll() {
  return send({ enable_all: !enabled.value });
}

// Идентификатор выводим из вендора: он же уходит в имя сервера, и держать два
// разных названия одного и того же незачем
function save() {
  const vendor = draft.vendor.trim();
  const url = draft.url.trim();
  if (!vendor || !url) {
    formError.value = 'нужны вендор и ссылка';
    return;
  }
  return send({
    source: {
      id: vendor.toLowerCase().replace(/[^a-z0-9]+/g, '-'),
      vendor,
      url,
      device_id: draft.device_id.trim(),
      max_clients: Number(draft.max_clients) || 0,
      enabled: true,
    },
  }).then(() => {
    draft.vendor = '';
    draft.url = '';
    draft.device_id = '';
    draft.max_clients = 0;
  });
}

function toggle(source) {
  return send({ source: { ...source, enabled: !source.enabled } });
}

function remove(source) {
  return send({ remove_id: source.id });
}

function when(unix) {
  if (!unix) return '';
  return new Date(Number(unix) * 1000).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' });
}
</script>
