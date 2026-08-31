<template>
  <div v-if="inviter.loaded && inviter.valid" class="invite-hero">
    <img :src="avatar" alt="" class="invite-avatar" aria-hidden="true" />
    <p class="invite-kicker">Вас пригласил</p>
    <p class="invite-name">{{ inviter.username || 'администратор' }}</p>
    <p class="invite-sub">
      Аккаунты здесь заводят только по приглашению.
      <template v-if="inviter.remaining > 1"> По этому коду осталось мест: {{ inviter.remaining }}.</template>
    </p>
  </div>
  <p v-else-if="inviter.loaded && !inviter.valid" class="state-error">
    {{ inviter.reason || 'Приглашение недействительно' }}
  </p>
</template>

<script setup>
import { computed, onMounted, reactive, watch } from 'vue';

const props = defineProps({
  token: { type: String, default: '' },
});
const emit = defineEmits(['loaded']);

const inviter = reactive({
  loaded: false,
  valid: false,
  username: '',
  avatar_version: 0,
  admin_id: 0,
  reason: '',
  remaining: 0,
});

const avatar = computed(() =>
  inviter.avatar_version
    ? `/api/admin/avatars/${inviter.admin_id}.png?v=${inviter.avatar_version}`
    : '/img/avatar-default.png',
);

onMounted(load);
watch(() => props.token, load);

async function load() {
  if (!props.token) {
    inviter.loaded = false;
    return;
  }
  try {
    const res = await fetch(`/api/invite?invite=${encodeURIComponent(props.token)}`);
    if (!res.ok) return;
    Object.assign(inviter, await res.json(), { loaded: true });
    emit('loaded', { valid: inviter.valid, username: inviter.username });
  } catch {
    // Проверить не вышло - форма всё равно работает
  }
}

defineExpose({ inviter });
</script>
