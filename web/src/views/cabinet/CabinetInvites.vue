<template>
  <section class="surface-card">
    <h2 class="section-title">Приглашения</h2>
    <p class="admin-muted mt-1">Вход в федерацию только по коду. Приглашать могут те, кто сам отдал в неё сервер.</p>
    <p v-if="invites.reason" class="state-hint mt-2">{{ invites.reason }}</p>
    <p v-if="inviteError" class="state-error mt-2">{{ inviteError }}</p>
    <div v-if="invites.may_invite" class="actions-row mt-3">
      <SamsungButton :busy="inviteBusy" @click="createInvite">Создать код</SamsungButton>
    </div>
    <ul v-if="invites.list.length" class="cabinet-invites mt-4">
      <li v-for="it in invites.list" :key="it.token" class="cabinet-invite">
        <CopyableLink :value="inviteLink(it.token)" />
        <span class="admin-muted">{{ it.used_count || 0 }} из {{ it.max_uses || 1 }}</span>
      </li>
    </ul>
    <p v-else-if="loaded" class="admin-muted mt-4">Кодов пока нет.</p>
  </section>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import CopyableLink from '@/components/domain/CopyableLink.vue';

const invites = reactive({ list: [], may_invite: false, reason: '' });
const inviteBusy = ref(false);
const inviteError = ref('');
const loaded = ref(false);

onMounted(loadInvites);

function inviteLink(token) {
  return `${window.location.origin}/register?invite=${token}`;
}

async function loadInvites() {
  try {
    const res = await fetch('/api/admin/invites', { credentials: 'include' });
    if (!res.ok) throw new Error(await res.text());
    const body = await res.json();
    invites.list = body.invites || [];
    invites.may_invite = Boolean(body.may_invite);
    invites.reason = body.reason || '';
  } catch (err) {
    inviteError.value = err.message;
  } finally {
    loaded.value = true;
  }
}

async function createInvite() {
  inviteBusy.value = true;
  inviteError.value = '';
  try {
    const res = await fetch('/api/admin/invites', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.message || 'Не удалось создать код');
    await loadInvites();
  } catch (err) {
    inviteError.value = err.message;
  } finally {
    inviteBusy.value = false;
  }
}
</script>

<style scoped>
.cabinet-invites {
  display: grid;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.cabinet-invite {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}
</style>
