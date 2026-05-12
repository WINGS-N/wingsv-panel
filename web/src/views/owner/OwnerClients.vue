<template>
  <SamsungCard
    title="Все клиенты"
    subtitle="Кросс-админский список всех устройств. Клик по строке ведёт в карточку клиента."
  >
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <SamsungSectionLoader v-else-if="!loaded" />

    <table v-if="clients.length" class="admin-table">
      <thead>
        <tr>
          <th>Имя</th>
          <th>Владелец</th>
          <th>Устройство</th>
          <th>OS · WINGS V</th>
          <th>Статус</th>
          <th>Последний контакт</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="c in clients" :key="c.id" class="admin-row" @click="openClient(c.id)">
          <td>
            <div class="admin-row-name">{{ c.name }}</div>
            <div class="admin-muted"><code class="admin-mono">{{ c.id }}</code></div>
          </td>
          <td>{{ c.owner_username }}</td>
          <td>{{ c.device_model || "—" }}</td>
          <td>{{ c.os_version || "—" }} · {{ c.app_version || "—" }}</td>
          <td>
            <SamsungPill :variant="c.online ? 'online' : 'offline'">
              {{ c.online ? "Онлайн" : "Оффлайн" }}
            </SamsungPill>
          </td>
          <td>{{ formatTs(c.last_seen_at) }}</td>
        </tr>
      </tbody>
    </table>
    <p v-else-if="loaded" class="admin-muted mt-4">Клиентов пока нет.</p>
  </SamsungCard>
</template>

<script setup>
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import SamsungCard from "@/components/layout/SamsungCard.vue";
import SamsungPill from "@/components/layout/SamsungPill.vue";
import SamsungSectionLoader from "@/components/layout/SamsungSectionLoader.vue";

const router = useRouter();
const clients = ref([]);
const loaded = ref(false);
const loadError = ref("");

async function load() {
  try {
    const res = await fetch("/api/owner/clients", { credentials: "include" });
    if (!res.ok) throw new Error(await res.text());
    const body = await res.json();
    clients.value = body.clients || [];
  } catch (err) {
    loadError.value = err.message;
  } finally {
    loaded.value = true;
  }
}

function openClient(id) {
  router.push({ name: "admin-client-detail", params: { id } });
}

function formatTs(iso) {
  if (!iso || iso.startsWith("1970")) return "—";
  try {
    return new Date(iso).toLocaleString("ru-RU");
  } catch {
    return iso;
  }
}

onMounted(load);
</script>
