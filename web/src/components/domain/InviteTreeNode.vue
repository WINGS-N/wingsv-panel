<template>
  <div class="tree-node">
    <div class="tree-node-row">
      <!-- Аватар и есть кнопка раскрытия: у кого есть ветвь, у того на аватаре
           счётчик, и нажимать хочется именно туда -->
      <button
        type="button"
        class="tree-avatar-button"
        :aria-expanded="open"
        :title="member.username"
        @click="$emit('toggle', member.admin_id)"
      >
        <img :src="avatar" alt="" class="tree-avatar" :class="{ 'is-cut': member.suspended || member.cut }" />
        <span v-if="children.length" class="tree-avatar-count">{{ children.length }}</span>
      </button>

      <div class="min-w-0 flex-1">
        <div class="flex flex-wrap items-center gap-2">
          <span class="admin-mono text-[15px]">{{ member.username }}</span>
          <span v-if="member.role === 'owner'" class="admin-pill is-info">владелец</span>
          <span v-if="member.suspended" class="admin-pill is-offline">срезан</span>
          <span v-else-if="member.cut" class="admin-pill is-offline">под срезом выше</span>
        </div>
        <span class="mt-0.5 flex flex-wrap items-center gap-3 text-[13px] text-wings-muted">
          <span class="inline-flex items-center gap-1">
            <ArrowUpDown :size="13" aria-hidden="true" />{{ bytes(member.own_bytes) }}
          </span>
          <span v-if="member.subtree_admins" class="inline-flex items-center gap-1">
            <Users :size="13" aria-hidden="true" />{{ member.subtree_admins }} · {{ bytes(member.subtree_bytes) }}
          </span>
        </span>
      </div>

      <SamsungButton
        v-if="member.role !== 'owner' && !member.suspended"
        variant="ghost"
        :busy="busyID === member.admin_id"
        @click="$emit('cut', member)"
      >
        <template #icon><Scissors class="button-icon" aria-hidden="true" /></template>
        Срезать
      </SamsungButton>
      <SamsungButton
        v-else-if="member.suspended"
        variant="ghost"
        :busy="busyID === member.admin_id"
        @click="$emit('restore', member)"
      >
        <template #icon><RotateCcw class="button-icon" aria-hidden="true" /></template>
        Вернуть
      </SamsungButton>
    </div>

    <!-- Подробности по нажатию на аватар: в строке им места нет, а прятать их
         совсем - значит заставлять лезть в аудит -->
    <div v-if="open" class="tree-details">
      <span class="tree-detail"><span class="admin-muted">пришёл</span> {{ short(member.created_at) }} </span>
      <span v-if="member.subtree_clients" class="tree-detail"
        ><span class="admin-muted">клиентов в ветви</span> {{ member.subtree_clients }}
      </span>
      <span v-if="member.reason" class="tree-detail"
        ><span class="admin-muted">причина</span> {{ member.reason }}
      </span>
    </div>

    <!-- Ветвь ветви режется так же, как ветвь: узел рекурсивный, поэтому
         кнопка есть на каждом уровне -->
    <div v-if="open && children.length" class="tree-children">
      <InviteTreeNode
        v-for="child in children"
        :key="child.admin_id"
        :member="child"
        :all="all"
        :opened="opened"
        :busy-i-d="busyID"
        @toggle="$emit('toggle', $event)"
        @cut="$emit('cut', $event)"
        @restore="$emit('restore', $event)"
      />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { ArrowUpDown, RotateCcw, Scissors, Users } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import { formatBytes } from '@/utils/format';

const props = defineProps({
  member: { type: Object, required: true },
  all: { type: Array, required: true },
  opened: { type: Object, required: true },
  busyID: { type: Number, default: 0 },
});
defineEmits(['toggle', 'cut', 'restore']);

const children = computed(() =>
  props.all.filter((m) => m.invited_by === props.member.admin_id).sort((a, b) => a.username.localeCompare(b.username)),
);

const open = computed(() => Boolean(props.opened[props.member.admin_id]));

const avatar = computed(() =>
  props.member.avatar_version
    ? `/api/admin/avatars/${props.member.admin_id}.png?v=${props.member.avatar_version}`
    : '/img/avatar-default.png',
);

const bytes = (v) => formatBytes(v || 0);

function short(value) {
  if (!value) return '-';
  try {
    // Приходит unix в секундах, Date ждёт миллисекунды
    return new Date(value * 1000).toLocaleDateString('ru-RU', { dateStyle: 'medium' });
  } catch {
    return value;
  }
}
</script>
