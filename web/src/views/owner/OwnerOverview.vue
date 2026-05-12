<template>
  <section class="surface-card">
    <h2 class="section-title">Обзор</h2>
    <p class="body-copy">Состояние платформы и быстрый доступ к управлению.</p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <SamsungSectionLoader v-else-if="!stats" />
    <div v-if="stats" class="admin-stats">
      <div class="stat">
        <span class="stat-kicker">Администраторы</span>
        <span class="stat-value">{{ stats.admins_count }}</span>
        <span class="stat-meta">всего в системе</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Клиенты</span>
        <span class="stat-value">{{ stats.clients_count }}</span>
        <span class="stat-meta">{{ stats.clients_online }} online</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">WS-соединений</span>
        <span class="stat-value">{{ stats.ws_clients }}</span>
        <span class="stat-meta">админ-WS: {{ stats.ws_admins }}</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Аптайм</span>
        <span class="stat-value">{{ uptimeText }}</span>
        <span class="stat-meta">с {{ formatTs(stats.started_at) }}</span>
      </div>
    </div>
    <div class="keyvals">
      <div class="keyval">
        <span class="keyval-label">База данных</span>
        <span class="keyval-value">
          <SamsungPill :variant="stats?.db_ok ? 'online' : 'offline'">
            {{ stats?.db_ok ? "OK" : "ERROR" }}
          </SamsungPill>
        </span>
      </div>
      <div class="keyval">
        <span class="keyval-label">Версия</span>
        <span class="keyval-value font-mono">{{ stats?.version || "—" }}</span>
      </div>
      <div class="keyval">
        <span class="keyval-label">Режим регистрации</span>
        <span class="keyval-value">{{ registrationLabel }}</span>
      </div>
    </div>
    <div class="actions-row mt-6">
      <SamsungButton :to="{ name: 'owner-admins' }">
        <template #icon><UsersRound class="button-icon" aria-hidden="true" /></template>
        Администраторы
      </SamsungButton>
      <SamsungButton variant="secondary" :to="{ name: 'owner-audit' }">
        <template #icon><ClipboardList class="button-icon" aria-hidden="true" /></template>
        Аудит
      </SamsungButton>
    </div>
  </section>

  <SamsungCard v-if="admins.length" class="mt-6" title="Пользователи панели">
    <template #actions>
      <SamsungButton variant="secondary" :to="{ name: 'owner-admins' }">Все</SamsungButton>
    </template>
    <ul class="admin-list mt-4">
      <li v-for="a in admins" :key="a.id" class="session-row">
        <div>
          <strong class="session-row-actor">{{ a.username }}</strong>
          <SamsungPill :variant="a.role === 'owner' ? 'online' : 'offline'" class="ml-2">
            {{ a.role }}
          </SamsungPill>
        </div>
        <span class="session-row-meta">
          {{ a.clients_total }} клиентов · {{ a.clients_online }} online · с {{ formatTs(a.created_at) }}
        </span>
      </li>
    </ul>
  </SamsungCard>

  <SamsungCard v-if="audit.length" class="mt-6" title="Лента событий" subtitle="Последние действия в системе.">
    <template #actions>
      <SamsungButton variant="secondary" :to="{ name: 'owner-audit' }">Полный журнал</SamsungButton>
    </template>
    <ul class="admin-list mt-4">
      <li v-for="e in audit" :key="e.id" class="session-row">
        <div>
          <strong class="session-row-actor">{{ e.actor_username || "—" }}</strong>
          <code class="admin-mono ml-2">{{ e.action }}</code>
          <span v-if="e.message" class="admin-muted ml-2">{{ e.message }}</span>
        </div>
        <span class="session-row-meta">{{ formatTs(e.ts) }}</span>
      </li>
    </ul>
  </SamsungCard>
</template>

<script setup>
import { computed, onMounted, ref } from "vue";
import { ClipboardList, UsersRound } from "lucide-vue-next";
import { registrationState, refreshRegistrationStatus } from "@/stores/auth.js";
import SamsungButton from "@/components/layout/SamsungButton.vue";
import SamsungCard from "@/components/layout/SamsungCard.vue";
import SamsungPill from "@/components/layout/SamsungPill.vue";
import SamsungSectionLoader from "@/components/layout/SamsungSectionLoader.vue";

const stats = ref(null);
const admins = ref([]);
const audit = ref([]);
const loadError = ref("");

const uptimeText = computed(() => {
  if (!stats.value) return "—";
  const s = stats.value.uptime_seconds;
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  if (d > 0) return `${d}д ${h}ч`;
  const m = Math.floor((s % 3600) / 60);
  return `${h}ч ${m}м`;
});

const registrationLabel = computed(() => {
  switch (registrationState.value.mode) {
    case "open": return "Открытая";
    case "invite": return "По invite-токену";
    case "closed": return "Закрытая";
    default: return registrationState.value.mode || "—";
  }
});

function formatTs(iso) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("ru-RU");
  } catch {
    return iso;
  }
}

async function loadStats() {
  try {
    const res = await fetch("/api/owner/stats", { credentials: "include" });
    if (!res.ok) throw new Error(await res.text());
    stats.value = await res.json();
  } catch (err) {
    loadError.value = err.message || "Не удалось загрузить статистику";
  }
}

async function loadAdminsPreview() {
  try {
    const res = await fetch("/api/owner/admins", { credentials: "include" });
    if (res.ok) {
      const body = await res.json();
      admins.value = (body.admins || []).slice(0, 5);
    }
  } catch {}
}

async function loadAuditPreview() {
  try {
    const res = await fetch("/api/owner/audit?limit=8", { credentials: "include" });
    if (res.ok) {
      const body = await res.json();
      audit.value = body.entries || [];
    }
  } catch {}
}

onMounted(() => {
  refreshRegistrationStatus();
  loadStats();
  loadAdminsPreview();
  loadAuditPreview();
});
</script>
