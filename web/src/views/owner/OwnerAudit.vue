<template>
  <SamsungCard
    title="Аудит"
    subtitle="Лента действий: логин, регистрация, управление админами, действия с клиентами."
  >
    <div class="actions-row">
      <SamsungButton variant="secondary" :busy="loading" @click="load">
        <template #icon><RefreshCw class="button-icon" aria-hidden="true" /></template>
        {{ loading ? "Обновляем…" : "Обновить" }}
      </SamsungButton>
    </div>

    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <SamsungSectionLoader v-else-if="loading && !entries.length" />

    <ul v-if="entries.length" class="admin-list mt-4">
      <li v-for="e in entries" :key="e.id" class="session-row">
        <div>
          <strong class="session-row-actor">{{ e.actor_username || "—" }}</strong>
          <code class="admin-mono ml-2">{{ e.action }}</code>
          <span v-if="e.message" class="admin-muted ml-2">{{ e.message }}</span>
          <span v-if="e.target_id" class="admin-muted ml-2">→ {{ e.target_type }}:{{ e.target_id }}</span>
        </div>
        <span class="session-row-meta">{{ formatTs(e.ts) }} · {{ e.ip || "—" }}</span>
      </li>
    </ul>
    <p v-else-if="!loading" class="admin-muted mt-4">Лента пуста.</p>
  </SamsungCard>
</template>

<script setup>
import { onMounted, ref } from "vue";
import { RefreshCw } from "lucide-vue-next";
import SamsungButton from "@/components/layout/SamsungButton.vue";
import SamsungCard from "@/components/layout/SamsungCard.vue";
import SamsungSectionLoader from "@/components/layout/SamsungSectionLoader.vue";

const entries = ref([]);
const loading = ref(false);
const loadError = ref("");

async function load() {
  loading.value = true;
  loadError.value = "";
  try {
    const res = await fetch("/api/owner/audit?limit=200", { credentials: "include" });
    if (!res.ok) throw new Error(await res.text());
    const body = await res.json();
    entries.value = body.entries || [];
  } catch (err) {
    loadError.value = err.message;
  } finally {
    loading.value = false;
  }
}

function formatTs(iso) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("ru-RU");
  } catch {
    return iso;
  }
}

onMounted(load);
</script>
