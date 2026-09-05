<template>
  <section class="surface-card">
    <div class="federation-live-head">
      <h2 class="section-title">Разметка</h2>
      <span class="admin-pill" :class="ready ? 'is-online' : 'is-info'">
        {{ ready ? 'можно учить' : `проверено ${byHuman} / ${NEEDED}` }}
      </span>
    </div>
    <p class="body-copy body-copy-wide">
      На этих метках учится модель, которая режет людям доступ. Машина на пограничных случаях плавает, и её ошибка,
      попав в обучение, станет повторяться уверенно. Поэтому обвинения проверяет человек, и его приговор старше.
    </p>
    <p v-if="loadError" class="state-error mt-3">{{ loadError }}</p>
    <p v-else-if="note" class="state-hint mt-3">{{ note }}</p>

    <div v-if="enabled" class="admin-stats">
      <div class="stat">
        <span class="stat-kicker">Размечено машиной</span>
        <span class="stat-value">{{ byMachine }}</span>
        <span class="stat-meta">claude и правила</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Проверено человеком</span>
        <span class="stat-value">{{ byHuman }}</span>
        <span class="stat-meta">{{ byHuman }} / {{ NEEDED }} для обучения</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Всего снимков</span>
        <span class="stat-value">{{ total }}</span>
        <span class="stat-meta">{{ accusedOnly ? 'показаны обвинения' : 'показаны все' }}</span>
      </div>
    </div>

    <div v-if="enabled" class="actions-row mt-4">
      <SamsungButton :variant="accusedOnly ? 'primary' : 'ghost'" @click="toggleAccused">
        {{ accusedOnly ? 'Только обвинения' : 'Показывать все' }}
      </SamsungButton>
    </div>
  </section>

  <section v-if="enabled && labels.length" class="surface-card mt-6">
    <h2 class="section-title">Снимки</h2>
    <div class="fed-cards">
      <div v-for="row in labels" :key="row.id" class="fed-card">
        <div class="fed-card-head">
          <router-link
            class="fed-card-name"
            :to="{ name: 'owner-oracle-subject', params: { id: row.subject_id } }"
          >
            {{ row.subject_id }}
          </router-link>
          <span class="admin-pill shrink-0" :class="row.label === 1 ? 'is-offline' : 'is-online'">
            {{ row.label === 1 ? 'обвинение' : 'чисто' }}
          </span>
        </div>
        <div class="fed-card-facts">
          <div class="fed-card-fact">
            <span class="fed-card-fact-label">Разметил</span>
            <span class="fed-card-fact-value">{{ byLabel(row.label_by) }}</span>
          </div>
          <div class="fed-card-fact">
            <span class="fed-card-fact-label">Когда</span>
            <span class="fed-card-fact-value">{{ when(row.at_unix) }}</span>
          </div>
        </div>
        <p v-if="row.why" class="body-copy">{{ row.why }}</p>

        <!-- Числа, по которым решали. Доменов тут нет и не будет: модель видит
             только форму поведения, и человек проверяет ровно то же -->
        <div class="label-values">
          <span v-for="item in shown(row)" :key="item.name" class="label-value">
            <span class="label-value-name">{{ featureLabel(item.name) }}</span>
            <span class="label-value-number">{{ item.text }}</span>
          </span>
        </div>

        <div class="fed-node-actions">
          <SamsungButton
            v-if="row.label !== 0 || row.label_by !== 'human'"
            variant="ghost"
            :busy="busy === row.id"
            @click="judge(row, 0)"
          >
            Это обычный человек
          </SamsungButton>
          <SamsungButton
            v-if="row.label !== 1 || row.label_by !== 'human'"
            variant="ghost"
            :busy="busy === row.id"
            @click="judge(row, 1)"
          >
            Это злоупотребление
          </SamsungButton>
        </div>
      </div>
    </div>
    <SamsungPager v-model:page="page" :total="total" :per-page="PER_PAGE" />
  </section>

  <section v-else-if="enabled" class="surface-card mt-6">
    <p class="state-hint">
      {{ accusedOnly ? 'Обвинений пока нет - проверять нечего.' : 'Снимков пока нет: они копятся, пока люди ходят через ноды.' }}
    </p>
  </section>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungPager from '@/components/controls/SamsungPager.vue';
import { FEATURE_LABELS } from './oracleLabels.js';

// NEEDED - с какого числа проверенных человеком меток есть смысл учить модель.
// Меньше сотни это не выборка, а хуйня
const NEEDED = 100;
const PER_PAGE = 12;

const enabled = ref(false);
const labels = ref([]);
const total = ref(0);
const byMachine = ref(0);
const byHuman = ref(0);
const page = ref(1);
const accusedOnly = ref(true);
const busy = ref(0);
const loadError = ref('');
const note = ref('');

const ready = computed(() => byHuman.value >= NEEDED);

onMounted(load);
watch([page, accusedOnly], load);

async function load() {
  try {
    const params = new URLSearchParams({
      limit: String(PER_PAGE),
      offset: String((page.value - 1) * PER_PAGE),
    });
    if (accusedOnly.value) params.set('accused', '1');
    const res = await fetch(`/api/owner/federation/oracle/labels?${params}`, { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    apply(await res.json());
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  }
}

function apply(data) {
  enabled.value = Boolean(data.enabled);
  labels.value = data.labels || [];
  total.value = Number(data.total || 0);
  byMachine.value = Number(data.by_machine || 0);
  byHuman.value = Number(data.by_human || 0);
  note.value = data.error || '';
}

// Приговор человека перебивает машинный, поэтому список сразу перечитывается,
// иначе на экране висит вчерашнее мнение модели
async function judge(row, label) {
  busy.value = row.id;
  try {
    const res = await fetch('/api/owner/federation/oracle/labels', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: row.id, label }),
    });
    if (!res.ok) throw new Error(await res.text());
    loadError.value = '';
    await load();
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    busy.value = 0;
  }
}

function toggleAccused() {
  accusedOnly.value = !accusedOnly.value;
  page.value = 1;
}

// Нули не показываем, у любого снимка их большинство, и эта хуйня топит те
// несколько чисел, ради которых человек сюда и пришёл
function shown(row) {
  const values = row.values || {};
  return Object.keys(values)
    .filter((name) => Number(values[name]) !== 0)
    .sort()
    .map((name) => ({ name, text: format(Number(values[name])) }));
}

function format(value) {
  if (Number.isInteger(value)) return String(value);
  return value.toFixed(2).replace(/\.?0+$/, '');
}

function featureLabel(name) {
  return FEATURE_LABELS[name] || name;
}

function byLabel(who) {
  if (who === 'human') return 'человек';
  if (who === 'rules') return 'правила';
  if (who === 'claude') return 'модель';
  return who || '-';
}

function when(unix) {
  if (!unix) return '-';
  return new Date(Number(unix) * 1000).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' });
}
</script>

<style scoped>
.label-values {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 4px;
}

/* Ширину режем по карточке: имена признаков длинные, и на телефоне такая
   плашка вылезала за экран вместе со всей страницей */
.label-value {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  max-width: 100%;
  padding: 4px 10px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.05);
  font-size: 13px;
}

.label-value-name {
  min-width: 0;
  overflow-wrap: anywhere;
  color: rgba(252, 252, 252, 0.55);
}

.label-value-number {
  flex-shrink: 0;
  font-variant-numeric: tabular-nums;
}
</style>
