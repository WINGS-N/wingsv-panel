<template>
  <SamsungCard
    class="mt-6"
    title="VK Links"
    subtitle="Общий пул VK Links для ваших устройств. Конкретно эти ссылки экспортируются в конфигурациях VK TURN, созданных через панель (managed VK TURN)."
  >
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <ul class="admin-list mt-4">
      <li v-for="(link, i) in links" :key="i" class="session-row vk-link-row">
        <code class="admin-mono vk-link-value" :title="link">{{ link }}</code>
        <SamsungIconButton variant="danger" size="small" aria-label="Удалить" @click="removeLink(i)">
          <Trash2 class="button-icon" aria-hidden="true" />
        </SamsungIconButton>
      </li>
      <li v-if="!links.length" class="session-row">
        <span class="admin-muted">VK-ссылок пока нет.</span>
      </li>
    </ul>

    <div class="vk-link-add mt-3">
      <OneuiInput v-model.trim="draft" label="Новая VK-ссылка" placeholder="vk-link" @keyup.enter="addLink" />
      <SamsungButton variant="secondary" :disabled="!draft" @click="addLink">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        Добавить
      </SamsungButton>
    </div>

    <p v-if="saveError" class="state-error mt-2">{{ saveError }}</p>
    <div class="actions-row mt-3">
      <SamsungButton :busy="saving" :disabled="!dirty" @click="save">Сохранить</SamsungButton>
      <span v-if="savedTick" class="admin-muted">Сохранено</span>
    </div>
  </SamsungCard>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import { Plus, Trash2 } from 'lucide-vue-next';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungIconButton from '@/components/layout/SamsungIconButton.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';

const links = ref([]);
const saved = ref([]);
const draft = ref('');
const loadError = ref('');
const saveError = ref('');
const saving = ref(false);
const savedTick = ref(false);

const dirty = computed(() => JSON.stringify(links.value) !== JSON.stringify(saved.value));

async function load() {
  loadError.value = '';
  try {
    const res = await fetch('/api/admin/vk-links', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const body = await res.json();
    links.value = [...(body.links || [])];
    saved.value = [...links.value];
  } catch (err) {
    loadError.value = err.message || 'Не удалось загрузить VK Links';
  }
}

function addLink() {
  const v = draft.value.trim();
  if (!v || links.value.includes(v)) {
    draft.value = '';
    return;
  }
  links.value.push(v);
  draft.value = '';
}

function removeLink(i) {
  links.value.splice(i, 1);
}

async function save() {
  saving.value = true;
  saveError.value = '';
  savedTick.value = false;
  try {
    const res = await fetch('/api/admin/vk-links', {
      method: 'PUT',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ links: links.value }),
    });
    if (!res.ok) throw new Error(await res.text());
    const body = await res.json();
    links.value = [...(body.links || [])];
    saved.value = [...links.value];
    savedTick.value = true;
    setTimeout(() => (savedTick.value = false), 1500);
  } catch (err) {
    saveError.value = err.message || 'Не удалось сохранить';
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<style scoped>
.vk-link-row {
  gap: 0.75rem;
}
.vk-link-value {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}
.vk-link-add {
  display: flex;
  align-items: flex-end;
  gap: 0.75rem;
}
.vk-link-add > :first-child {
  flex: 1;
}
</style>
