<template>
  <SamsungCard
    title="Администраторы"
    subtitle="Создание, удаление и сброс паролей. Удаление каскадно убирает клиентов."
  >
    <template #actions>
      <SamsungButton @click="openCreate">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        Создать админа
      </SamsungButton>
    </template>

    <p v-if="loadError" class="state-error">{{ loadError }}</p>
    <SamsungSectionLoader v-else-if="!adminsLoaded" />

    <table v-if="admins.length" class="admin-table">
      <thead>
        <tr>
          <th>Логин</th>
          <th>Роль</th>
          <th>Клиентов</th>
          <th>Online</th>
          <th>Создан</th>
          <th>Последний вход</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="a in admins" :key="a.id" class="admin-row">
          <td data-label="Логин"><strong>{{ a.username }}</strong></td>
          <td data-label="Роль">
            <SamsungPill :variant="a.role === 'owner' ? 'online' : 'offline'">
              {{ a.role }}
            </SamsungPill>
          </td>
          <td data-label="Клиентов">{{ a.clients_total }}</td>
          <td data-label="Online">{{ a.clients_online }}</td>
          <td data-label="Создан">{{ formatTs(a.created_at) }}</td>
          <td data-label="Последний вход">{{ formatTs(a.last_login_at) }}</td>
          <td class="admin-row-actions" data-label="Действия">
            <SamsungIconButton
              :title="'Сменить пароль'"
              :aria-label="`Сменить пароль ${a.username}`"
              @click="openReset(a)"
            >
              <KeyRound class="h-4 w-4" aria-hidden="true" />
            </SamsungIconButton>
            <SamsungIconButton
              v-if="a.role !== 'owner'"
              variant="danger"
              :busy="deletingId === a.id"
              :title="'Удалить'"
              :aria-label="`Удалить ${a.username}`"
              @click="askDelete(a)"
            >
              <Trash2 class="h-4 w-4" aria-hidden="true" />
            </SamsungIconButton>
          </td>
        </tr>
      </tbody>
    </table>

    <h3 class="admin-section-subtitle mt-6">Регистрация</h3>
    <div class="mt-2">
      <OneuiRadioGroup
        :model-value="registrationState.mode"
        :options="regOptions"
        variant="pill"
        @update:model-value="setRegistrationMode"
      />
    </div>

    <div v-if="registrationState.mode === 'invite'" class="mt-4">
      <div class="actions-row">
        <SamsungButton variant="secondary" @click="onCreateInvite">
          <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
          Создать токен
        </SamsungButton>
      </div>
      <ul v-if="invites.length" class="admin-list mt-3">
        <li v-for="it in invites" :key="it.token" class="admin-list-item">
          <div class="admin-list-row">
            <div class="admin-list-text">
              <code class="admin-mono">{{ it.token }}</code>
              <span class="admin-muted">создан {{ formatTs(it.created_at) }}</span>
              <span v-if="it.used" class="admin-muted">использован {{ formatTs(it.used_at) }}</span>
            </div>
            <div class="admin-list-actions">
              <SamsungIconButton size="small" :title="'Скопировать'" @click="copyInvite(it.token)">
                <Copy class="button-icon" aria-hidden="true" />
              </SamsungIconButton>
              <SamsungIconButton size="small" :title="'Удалить'" @click="onDeleteInvite(it.token)">
                <Trash2 class="button-icon" aria-hidden="true" />
              </SamsungIconButton>
            </div>
          </div>
        </li>
      </ul>
    </div>

    <SamsungModal v-model="showCreate" title="Новый админ">
      <OneuiInput v-model.trim="newUsername" label="Логин" class="mt-4" />
      <div class="mt-3">
        <OneuiInput v-model="newPassword" label="Пароль" type="text" autocomplete="off" />
      </div>
      <p class="admin-muted mt-2">Минимум 8 символов. Админ обязан сменить пароль при первом входе.</p>
      <p v-if="createError" class="state-error mt-3">{{ createError }}</p>
      <template #actions>
        <SamsungButton :busy="creating" :disabled="!canCreate" @click="onCreateAdmin">
          <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
          {{ creating ? "Создаём…" : "Создать" }}
        </SamsungButton>
        <SamsungButton variant="secondary" @click="closeCreate">
          <template #icon><X class="button-icon" aria-hidden="true" /></template>
          Отмена
        </SamsungButton>
      </template>
    </SamsungModal>

    <SamsungModal
      :model-value="!!confirmDelete"
      :busy="!!deletingId"
      title="Удалить администратора?"
      @update:model-value="cancelDelete"
    >
      <p v-if="confirmDelete" class="body-copy">
        Аккаунт <strong>{{ confirmDelete.username }}</strong> будет удалён. Каскадно удалятся все
        принадлежащие ему клиенты, их конфигурации и журналы. Действие необратимо.
      </p>
      <p v-if="deleteError" class="state-error mt-3">{{ deleteError }}</p>
      <template #actions>
        <SamsungButton
          variant="danger"
          :busy="deletingId === confirmDelete?.id"
          @click="performDelete"
        >
          <template #icon><Trash2 class="button-icon" aria-hidden="true" /></template>
          {{ deletingId === confirmDelete?.id ? "Удаляем…" : "Удалить" }}
        </SamsungButton>
        <SamsungButton
          variant="secondary"
          :disabled="deletingId === confirmDelete?.id"
          @click="cancelDelete"
        >
          <template #icon><X class="button-icon" aria-hidden="true" /></template>
          Отмена
        </SamsungButton>
      </template>
    </SamsungModal>

    <SamsungModal
      :model-value="!!resetTarget"
      :title="resetTarget ? `Сменить пароль · ${resetTarget.username}` : ''"
      @update:model-value="resetTarget = null"
    >
      <OneuiInput
        v-model="resetPassword"
        label="Новый пароль"
        type="text"
        autocomplete="off"
        class="mt-4"
      />
      <p v-if="resetError" class="state-error mt-3">{{ resetError }}</p>
      <template #actions>
        <SamsungButton :busy="resetting" :disabled="!resetPassword" @click="onResetSubmit">
          <template #icon><KeyRound class="button-icon" aria-hidden="true" /></template>
          {{ resetting ? "Меняем…" : "Сменить" }}
        </SamsungButton>
        <SamsungButton variant="secondary" @click="resetTarget = null">
          <template #icon><X class="button-icon" aria-hidden="true" /></template>
          Отмена
        </SamsungButton>
      </template>
    </SamsungModal>
  </SamsungCard>
</template>

<script setup>
import { computed, onMounted, ref } from "vue";
import { Clock, Copy, Eye, KeyRound, Plus, Trash2, X, Infinity as InfinityIcon } from "lucide-vue-next";
import { registrationState, refreshRegistrationStatus } from "@/stores/auth.js";
import OneuiInput from "@/components/controls/OneuiInput.vue";
import OneuiRadioGroup from "@/components/controls/OneuiRadioGroup.vue";
import SamsungButton from "@/components/layout/SamsungButton.vue";
import SamsungCard from "@/components/layout/SamsungCard.vue";
import SamsungIconButton from "@/components/layout/SamsungIconButton.vue";
import SamsungModal from "@/components/layout/SamsungModal.vue";
import SamsungPill from "@/components/layout/SamsungPill.vue";
import SamsungSectionLoader from "@/components/layout/SamsungSectionLoader.vue";

const admins = ref([]);
const adminsLoaded = ref(false);
const invites = ref([]);
const loadError = ref("");
const showCreate = ref(false);
const newUsername = ref("");
const newPassword = ref("");
const creating = ref(false);
const createError = ref("");
const resetTarget = ref(null);
const resetPassword = ref("");
const resetting = ref(false);
const resetError = ref("");
const confirmDelete = ref(null);
const deletingId = ref(0);
const deleteError = ref("");

const canCreate = computed(() => newUsername.value && newPassword.value && newPassword.value.length >= 8);

const regOptions = [
  { value: "open", label: "Открытая", icon: InfinityIcon },
  { value: "invite", label: "По invite", icon: Eye },
  { value: "closed", label: "Закрытая", icon: Clock },
];

async function loadAdmins() {
  try {
    const res = await fetch("/api/owner/admins", { credentials: "include" });
    if (!res.ok) throw new Error(await res.text());
    const body = await res.json();
    admins.value = body.admins || [];
  } catch (err) {
    loadError.value = err.message;
  } finally {
    adminsLoaded.value = true;
  }
}

async function loadInvites() {
  try {
    const res = await fetch("/api/owner/invites", { credentials: "include" });
    if (res.ok) {
      const body = await res.json();
      invites.value = body.invites || [];
    }
  } catch {}
}

function openCreate() {
  newUsername.value = "";
  newPassword.value = "";
  createError.value = "";
  showCreate.value = true;
}

function closeCreate() {
  showCreate.value = false;
}

async function onCreateAdmin() {
  if (!canCreate.value || creating.value) return;
  creating.value = true;
  createError.value = "";
  try {
    const res = await fetch("/api/owner/admins", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: newUsername.value, password: newPassword.value }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || "Не удалось создать");
    }
    closeCreate();
    await loadAdmins();
  } catch (err) {
    createError.value = err.message;
  } finally {
    creating.value = false;
  }
}

function askDelete(admin) {
  confirmDelete.value = { id: admin.id, username: admin.username };
  deleteError.value = "";
}

function cancelDelete() {
  if (deletingId.value) return;
  confirmDelete.value = null;
  deleteError.value = "";
}

async function performDelete() {
  if (!confirmDelete.value) return;
  const target = confirmDelete.value;
  deletingId.value = target.id;
  deleteError.value = "";
  try {
    const res = await fetch(`/api/owner/admins/${target.id}`, {
      method: "DELETE",
      credentials: "include",
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || "Не удалось удалить");
    }
    confirmDelete.value = null;
    await loadAdmins();
  } catch (err) {
    deleteError.value = err.message || "Не удалось удалить";
  } finally {
    deletingId.value = 0;
  }
}

function openReset(admin) {
  resetTarget.value = admin;
  resetPassword.value = "";
  resetError.value = "";
}

async function onResetSubmit() {
  if (!resetTarget.value || !resetPassword.value) return;
  resetting.value = true;
  resetError.value = "";
  try {
    const res = await fetch(`/api/owner/admins/${resetTarget.value.id}/reset-password`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ new_password: resetPassword.value }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || "Не удалось");
    }
    resetTarget.value = null;
  } catch (err) {
    resetError.value = err.message;
  } finally {
    resetting.value = false;
  }
}

async function setRegistrationMode(mode) {
  if (registrationState.value.mode === mode) return;
  try {
    const res = await fetch("/api/owner/settings", {
      method: "PUT",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ registration_mode: mode }),
    });
    if (!res.ok) throw new Error(await res.text());
    registrationState.value = { mode, loaded: true };
    if (mode === "invite") loadInvites();
  } catch (err) {
    loadError.value = err.message;
  }
}

async function onCreateInvite() {
  try {
    const res = await fetch("/api/owner/invites", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ttl_hours: 168 }),
    });
    if (!res.ok) throw new Error(await res.text());
    await loadInvites();
  } catch (err) {
    loadError.value = err.message;
  }
}

async function onDeleteInvite(token) {
  try {
    await fetch(`/api/owner/invites/${token}`, { method: "DELETE", credentials: "include" });
    await loadInvites();
  } catch {}
}

async function copyInvite(token) {
  try {
    await navigator.clipboard.writeText(token);
  } catch {}
}

function formatTs(iso) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("ru-RU");
  } catch {
    return iso;
  }
}

onMounted(() => {
  refreshRegistrationStatus();
  loadAdmins();
  loadInvites();
});
</script>
