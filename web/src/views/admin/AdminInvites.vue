<template>
  <section class="surface-card">
    <h2 class="section-title">Приглашения</h2>
    <p class="body-copy body-copy-wide">
      Зарегистрироваться можно только по коду. Один код можно выдать одному человеку или сразу нескольким - счётчик
      ведётся по тем, кто пришёл именно по нему.
    </p>
    <p v-if="loadError" class="state-error mt-3">{{ loadError }}</p>
    <p v-if="!mayInvite && blockReason" class="state-hint">{{ blockReason }}</p>

    <!-- Код можно ввести и после регистрации: аккаунт, заведённый раньше
         приглашения, иначе остаётся вне дерева навсегда -->
    <div v-if="!mayInvite" class="entry-card mt-4">
      <p class="body-copy">Вас кто-то пригласил? Введите код - он поставит вас в дерево и откроет приглашения.</p>
      <div class="form-grid mt-3">
        <OneuiInput v-model.trim="redeemCode" label="Код приглашения" placeholder="например, 9f3a1c" />
      </div>
      <!-- Кто пригласил, видно до применения: код - это чужая ссылка, и человек
           должен понимать, в чьё дерево встаёт -->
      <InviteHero :token="redeemCode" class="mt-3" />
      <div class="actions-row">
        <SamsungButton :busy="redeeming" @click="redeem">
          <template #icon><Ticket class="button-icon" aria-hidden="true" /></template>
          Применить код
        </SamsungButton>
      </div>
      <p v-if="redeemError" class="state-error">{{ redeemError }}</p>
    </div>

    <!-- Поля стоят полями, а не втиснуты в строку рядом с кнопкой: подпись над
         вводом и кнопка снизу - то, как выглядит любая форма в этой панели -->
    <div v-if="mayInvite" class="form-grid mt-5">
      <OneuiInput v-model.number="maxUses" label="Человек по коду" type="number" :min="1" :max="50" />
      <OneuiInput v-model.number="ttlHours" label="Живёт часов (0 - без срока)" type="number" :min="0" :max="8760" />
    </div>

    <div v-if="mayInvite" class="actions-row">
      <SamsungButton :busy="creating" @click="create">
        <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
        Выписать код
      </SamsungButton>
    </div>

    <div v-if="invites.length" class="fed-cards">
      <div v-for="it in invites" :key="it.token" class="fed-card">
        <div class="fed-card-head">
          <span class="admin-mono min-w-0 flex-1 truncate text-[15px]">{{ it.token }}</span>
          <span class="admin-pill shrink-0" :class="it.spent ? 'is-offline' : 'is-online'">
            {{ it.spent ? 'исчерпан' : 'годен' }}
          </span>
        </div>
        <div class="fed-card-facts">
          <div class="fed-card-fact">
            <span class="fed-card-fact-label">Использован</span>
            <span class="fed-card-fact-value">
              {{ it.use_count }}
              <span class="text-wings-kicker">/ {{ it.max_uses ? it.max_uses : 'без потолка' }}</span>
            </span>
          </div>
          <div class="fed-card-fact">
            <span class="fed-card-fact-label">Срок</span>
            <span class="fed-card-fact-value">{{ it.expires_at ? short(it.expires_at) : 'без срока' }}</span>
          </div>
        </div>
        <CopyableValue label="Ссылка" :value="it.link" />
        <div class="fed-node-actions">
          <SamsungButton variant="ghost" :busy="revoking === it.token" @click="revoke(it)">
            <template #icon><Trash2 class="button-icon" aria-hidden="true" /></template>
            Отозвать
          </SamsungButton>
        </div>
      </div>
    </div>
    <p v-else-if="!loading" class="admin-muted mt-4">Вы пока никого не приглашали.</p>
  </section>
</template>

<script setup>
import { onMounted, ref } from 'vue';
import { Plus, Ticket, Trash2 } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import CopyableValue from '@/components/domain/CopyableValue.vue';
import InviteHero from '@/components/domain/InviteHero.vue';

const invites = ref([]);
const loading = ref(false);
const creating = ref(false);
const loadError = ref('');
const maxUses = ref(1);
const ttlHours = ref(0);
const mayInvite = ref(true);
const blockReason = ref('');
const redeemCode = ref('');
const redeeming = ref(false);
const redeemError = ref('');
const revoking = ref('');

onMounted(load);

async function load() {
  loading.value = true;
  try {
    const res = await fetch('/api/admin/invites', { credentials: 'include' });
    if (!res.ok) throw new Error(await errorText(res));
    const data = await res.json();
    invites.value = data.invites || [];
    mayInvite.value = data.may_invite !== false;
    blockReason.value = data.reason || '';
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    loading.value = false;
  }
}

// Отзыв бьёт только по будущим регистрациям: те, кто уже пришёл, остаются в
// дереве, и обрезать ветку - отдельное право владельца
async function revoke(invite) {
  revoking.value = invite.token;
  try {
    const res = await fetch(`/api/admin/invites/${encodeURIComponent(invite.token)}`, {
      method: 'DELETE',
      credentials: 'include',
    });
    if (!res.ok) throw new Error(await errorText(res));
    await load();
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    revoking.value = '';
  }
}

async function redeem() {
  redeeming.value = true;
  redeemError.value = '';
  try {
    const res = await fetch('/api/admin/invites/redeem', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token: redeemCode.value }),
    });
    if (!res.ok) throw new Error(await errorText(res));
    redeemCode.value = '';
    await load();
  } catch (err) {
    redeemError.value = String(err.message || err);
  } finally {
    redeeming.value = false;
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

// Приложение ловит эту схему и применяет код само, без копипасты руками

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
