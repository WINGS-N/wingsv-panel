<template>
  <li class="tree-item">
    <!-- Аватар и есть узел графа: имя под ним, связи рисуются линиями от
         родителя, а подробности раскрываются по нажатию - в самом дереве им
         места нет, а прятать их совсем значит гонять человека в аудит -->
    <div class="tree-person">
      <button
        type="button"
        class="tree-avatar-button"
        :class="{ 'is-open': open }"
        :aria-expanded="open"
        :title="member.username"
        @click="$emit('toggle', member.admin_id)"
      >
        <img :src="avatar" alt="" class="tree-avatar" :class="{ 'is-cut': member.suspended || member.cut }" />
        <span v-if="children.length" class="tree-avatar-count">{{ children.length }}</span>
      </button>
      <span class="tree-name">{{ member.username }}</span>
      <span class="tree-meta">{{ bytes(member.own_bytes) }}</span>
      <span v-if="member.role === 'owner'" class="tree-tag is-owner">владелец</span>
      <span v-else-if="member.suspended" class="tree-tag is-cut">срезан</span>
      <span v-else-if="member.cut" class="tree-tag is-cut">под срезом</span>
    </div>

    <div v-if="open" class="tree-card">
      <div class="tree-card-rows">
        <span><span class="admin-muted">пришёл</span> {{ short(member.created_at) }}</span>
        <span v-if="member.subtree_admins">
          <span class="admin-muted">в ветви</span> {{ member.subtree_admins }} чел.,
          {{ bytes(member.subtree_bytes) }}
        </span>
        <span v-if="member.reason"><span class="admin-muted">причина</span> {{ member.reason }}</span>
      </div>
      <!-- Ветвь ветви режется так же, как ветвь: узел рекурсивный, поэтому
           действие есть на каждом уровне -->
      <SamsungButton
        v-if="member.role !== 'owner' && !member.suspended"
        variant="ghost"
        :busy="busyID === member.admin_id"
        @click="$emit('cut', member)"
      >
        <template #icon><Scissors class="button-icon" aria-hidden="true" /></template>
        Срезать ветвь
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

    <ul v-if="children.length" class="tree-children">
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
    </ul>
  </li>
</template>

<script setup>
import { computed } from 'vue';
import { RotateCcw, Scissors } from 'lucide-vue-next';
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
