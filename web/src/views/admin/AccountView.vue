<template>
  <section class="admin-card">
    <h1 class="admin-card-title">Аккаунт администратора</h1>
    <p class="admin-muted">Имя пользователя: <strong>{{ admin?.username || "—" }}</strong></p>

    <h2 class="admin-section-subtitle mt-5">Аватар</h2>
    <div class="avatar-row">
      <span class="avatar-preview" aria-hidden="true">
        <img :src="myAvatarUrl" alt="" />
      </span>
      <div class="avatar-actions">
        <input
          ref="fileInput"
          type="file"
          accept="image/png,image/jpeg,image/webp"
          class="hidden"
          @change="onFilePicked"
        />
        <SamsungButton variant="secondary" :busy="avatarBusy" @click="fileInput?.click()">
          <template #icon><Camera class="button-icon" aria-hidden="true" /></template>
          {{ avatarBusy ? "Загружаем…" : "Загрузить" }}
        </SamsungButton>
        <SamsungButton
          v-if="hasCustomAvatar"
          variant="secondary"
          :disabled="avatarBusy"
          @click="removeAvatar"
        >
          <template #icon><Trash2 class="button-icon" aria-hidden="true" /></template>
          Сбросить
        </SamsungButton>
      </div>
    </div>
    <p v-if="avatarError" class="admin-error mt-2">{{ avatarError }}</p>
    <p class="admin-muted mt-2">PNG / JPEG / WebP, до 2 МБ.</p>

    <h2 class="admin-section-subtitle mt-6">Сменить пароль</h2>
    <form class="admin-account-form" @submit.prevent="onSubmit">
      <OneuiInput
        v-model="oldPassword"
        label="Текущий пароль"
        type="password"
        autocomplete="current-password"
      />
      <div class="mt-3">
        <OneuiInput
          v-model="newPassword"
          label="Новый пароль"
          type="password"
          autocomplete="new-password"
        />
      </div>
      <div class="mt-3">
        <OneuiInput
          v-model="newPassword2"
          label="Повтор"
          type="password"
          autocomplete="new-password"
        />
      </div>
      <p v-if="error" class="admin-error mt-3">{{ error }}</p>
      <p v-if="ok" class="admin-success mt-3">Пароль обновлён.</p>
      <div class="actions-row mt-4">
        <SamsungButton type="submit" :busy="busy" :disabled="!canSubmit">
          <template #icon><KeyRound class="button-icon" aria-hidden="true" /></template>
          {{ busy ? "Сохраняем…" : "Сменить пароль" }}
        </SamsungButton>
      </div>
    </form>
  </section>
</template>

<script setup>
import { computed, ref } from "vue";
import { Camera, KeyRound, Trash2 } from "lucide-vue-next";
import { authState, changePassword, myAvatarUrl, refreshSession } from "@/stores/auth.js";
import OneuiInput from "@/components/controls/OneuiInput.vue";
import SamsungButton from "@/components/layout/SamsungButton.vue";

const admin = computed(() => authState.value.admin);
const oldPassword = ref("");
const newPassword = ref("");
const newPassword2 = ref("");
const busy = ref(false);
const error = ref("");
const ok = ref(false);

const fileInput = ref(null);
const avatarBusy = ref(false);
const avatarError = ref("");
const hasCustomAvatar = computed(() => (admin.value?.avatar_version || 0) > 0);

const canSubmit = computed(() => oldPassword.value && newPassword.value && newPassword.value === newPassword2.value);

async function onSubmit() {
  if (!canSubmit.value) {
    error.value = "Новый пароль и повтор не совпадают";
    return;
  }
  busy.value = true;
  error.value = "";
  ok.value = false;
  try {
    await changePassword(oldPassword.value, newPassword.value);
    ok.value = true;
    oldPassword.value = "";
    newPassword.value = "";
    newPassword2.value = "";
  } catch (err) {
    error.value = err.message || "Не удалось сменить пароль";
  } finally {
    busy.value = false;
  }
}

async function onFilePicked(e) {
  const file = e.target.files?.[0];
  e.target.value = "";
  if (!file) return;
  if (file.size > 2 * 1024 * 1024) {
    avatarError.value = "Файл слишком большой (макс. 2 МБ)";
    return;
  }
  avatarError.value = "";
  avatarBusy.value = true;
  try {
    const form = new FormData();
    form.append("avatar", file);
    const res = await fetch("/api/admin/me/avatar", {
      method: "POST",
      credentials: "include",
      body: form,
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || "Не удалось загрузить");
    }
    await refreshSession();
  } catch (err) {
    avatarError.value = err.message;
  } finally {
    avatarBusy.value = false;
  }
}

async function removeAvatar() {
  avatarBusy.value = true;
  avatarError.value = "";
  try {
    await fetch("/api/admin/me/avatar", { method: "DELETE", credentials: "include" });
    await refreshSession();
  } catch (err) {
    avatarError.value = err.message;
  } finally {
    avatarBusy.value = false;
  }
}
</script>

<style scoped>
.avatar-row {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-top: 12px;
}

.avatar-preview {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 96px;
  height: 96px;
  border-radius: 9999px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.avatar-preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.avatar-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.hidden {
  display: none;
}
</style>
