<template>
  <section class="surface-card">
    <h2 class="section-title">Аккаунт</h2>

    <h3 class="admin-section-subtitle mt-4">Пароль</h3>
    <p v-if="passwordOk" class="state-hint">Пароль сменён.</p>
    <p v-if="passwordError" class="state-error">{{ passwordError }}</p>
    <form class="form-grid mt-3" @submit.prevent="submitPassword">
      <OneuiInput v-model="passwords.old" label="Текущий пароль" type="password" autocomplete="current-password" />
      <OneuiInput v-model="passwords.next" label="Новый пароль" type="password" autocomplete="new-password" />
      <OneuiInput v-model="passwords.repeat" label="Повторите новый" type="password" autocomplete="new-password" />
    </form>
    <div class="actions-row mt-3">
      <SamsungButton :busy="passwordBusy" :disabled="!passwordReady" @click="submitPassword">
        Сменить пароль
      </SamsungButton>
    </div>

    <h3 class="admin-section-subtitle mt-6">Управление клиентами</h3>
    <template v-if="hasPanel">
      <p class="admin-muted">Панель вам открыта.</p>
      <div class="actions-row mt-3">
        <SamsungButton variant="ghost" @click="router.push({ name: 'admin-clients' })">Открыть панель</SamsungButton>
      </div>
    </template>
    <template v-else>
      <p class="admin-muted">Своих клиентов заводят в админ-панели. Откройте её себе, когда понадобится.</p>
      <p v-if="panelError" class="state-error mt-2">{{ panelError }}</p>
      <p v-if="panelRequested" class="state-hint mt-2">Заявка отправлена, ждём решения владельца.</p>
      <div v-else class="actions-row mt-3">
        <SamsungButton :busy="panelBusy" @click="openPanel">Стать администратором</SamsungButton>
      </div>
    </template>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { authState, changePassword, refreshSession } from '@/stores/auth.js';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';

const router = useRouter();

const passwords = reactive({ old: '', next: '', repeat: '' });
const passwordBusy = ref(false);
const passwordError = ref('');
const passwordOk = ref(false);
const passwordReady = computed(
  () => passwords.old && passwords.next && passwords.next === passwords.repeat && !passwordBusy.value,
);

const panelBusy = ref(false);
const panelError = ref('');
const panelRequested = ref(false);
const hasPanel = computed(() => Boolean(authState.value.admin?.panel_access));

onMounted(async () => {
  // Статус заявки живёт в сессии: кнопку нельзя показывать тому, кто уже попросил
  await refreshSession();
  panelRequested.value = Boolean(authState.value.admin?.panel_requested);
});

async function submitPassword() {
  if (!passwordReady.value) return;
  passwordBusy.value = true;
  passwordError.value = '';
  passwordOk.value = false;
  try {
    await changePassword(passwords.old, passwords.next);
    passwordOk.value = true;
    passwords.old = '';
    passwords.next = '';
    passwords.repeat = '';
  } catch (err) {
    passwordError.value = err.message || 'Не удалось сменить пароль';
  } finally {
    passwordBusy.value = false;
  }
}

async function openPanel() {
  panelBusy.value = true;
  panelError.value = '';
  try {
    const res = await fetch('/api/admin/me/panel-access', { method: 'POST', credentials: 'include' });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.message || 'Не удалось открыть панель');
    // Когда владелец включил модерацию, доступ не выдаётся сразу
    panelRequested.value = Boolean(body.requested);
    if (body.granted) {
      await refreshSession();
      router.push({ name: 'admin-clients' });
    }
  } catch (err) {
    panelError.value = err.message;
  } finally {
    panelBusy.value = false;
  }
}
</script>
