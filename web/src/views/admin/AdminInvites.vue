<template>
  <section class="surface-card">
    <h2 class="section-title">Приглашения</h2>
    <p class="body-copy body-copy-wide">
      Вход в панель только по приглашению - этим и держится дерево. Код можно выдать одному человеку или сразу
      нескольким; лимит считает тех, кто пришёл по самому коду, а сколько людей приведут они дальше - уже их дело.
    </p>
    <p v-if="loadError" class="state-error mt-3">{{ loadError }}</p>

    <!-- Поля стоят полями, а не втиснуты в строку рядом с кнопкой: подпись над
         вводом и кнопка снизу - то, как выглядит любая форма в этой панели -->
    <div class="form-grid mt-5">
      <OneuiInput v-model.number="maxUses" label="Человек по коду" type="number" :min="1" :max="50" />
      <OneuiInput v-model.number="ttlHours" label="Живёт часов (0 - без срока)" type="number" :min="0" :max="8760" />
    </div>

    <div class="actions-row">
      <SamsungButton :busy="creating" @click="create">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        Выписать код
      </SamsungButton>
    </div>

    <div v-if="invites.length" class="fed-node-list mt-4">
      <div v-for="it in invites" :key="it.token" class="fed-node-row">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="admin-mono text-[15px]">{{ it.token }}</span>
            <span class="admin-pill" :class="it.spent ? 'is-offline' : 'is-online'">
              {{ it.spent ? 'исчерпан' : 'годен' }}
            </span>
          </div>
          <span class="mt-1 flex flex-wrap items-center gap-4 text-[13px] text-wings-muted">
            <span class="inline-flex items-center gap-1">
              <Users :size="13" aria-hidden="true" />{{ it.use_count }} / {{ it.max_uses }}
            </span>
            <span class="inline-flex items-center gap-1">
              <CalendarRange :size="13" aria-hidden="true" />
              {{ it.expires_at ? `до ${short(it.expires_at)}` : 'без срока' }}
            </span>
          </span>
          <CopyableLink :value="it.link" class="mt-2" />
        </div>
      </div>
    </div>
    <p v-else-if="!loading" class="admin-muted mt-4">Вы пока никого не приглашали.</p>
  </section>
</template>

<script setup>
import { onMounted, ref } from 'vue';
import { CalendarRange, Plus, Users } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import CopyableLink from '@/components/domain/CopyableLink.vue';

const invites = ref([]);
const loading = ref(false);
const creating = ref(false);
const loadError = ref('');
const maxUses = ref(1);
const ttlHours = ref(0);

onMounted(load);

async function load() {
  loading.value = true;
  try {
    const res = await fetch('/api/admin/invites', { credentials: 'include' });
    if (!res.ok) throw new Error(await errorText(res));
    invites.value = (await res.json()).invites || [];
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    loading.value = false;
  }
}

async function create() {
  creating.value = true;
  try {
    const res = await fetch('/api/admin/invites', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        max_uses: Math.max(1, Number(maxUses.value) || 1),
        ttl_hours: Math.max(0, Number(ttlHours.value) || 0),
      }),
    });
    if (!res.ok) throw new Error(await errorText(res));
    loadError.value = '';
    await load();
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    creating.value = false;
  }
}

function short(value) {
  try {
    return new Date(value).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' });
  } catch {
    return value;
  }
}

async function errorText(res) {
  try {
    return (await res.json()).message || `HTTP ${res.status}`;
  } catch {
    return `HTTP ${res.status}`;
  }
}
</script>
