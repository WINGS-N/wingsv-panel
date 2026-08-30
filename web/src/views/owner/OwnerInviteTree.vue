<template>
  <section class="surface-card">
    <h2 class="section-title">Дерево приглашений</h2>
    <p class="body-copy">
      Кто кого привёл. Ветвь можно срезать в любой точке — вместе со срезанным уходят все, кого он пригласил, и их
      сессии закрываются сразу. Трафик по поддереву — только наблюдение: он никого не отрезает, режет лишь личный лимит.
    </p>
    <p v-if="loadError" class="state-error">{{ loadError }}</p>

    <!-- Ветви и одиночки разделены нарочно. Пока никто никого не пригласил,
         каждый администратор - сам себе корень, и список из полусотни корней
         читается как свалка, а не как дерево. -->
    <div v-if="roots.length" class="tree-root mt-4">
      <InviteTreeNode
        v-for="m in roots"
        :key="m.admin_id"
        :member="m"
        :all="members"
        :opened="opened"
        :busy-i-d="busyID"
        @toggle="toggle"
        @cut="cut"
        @restore="restore"
      />
    </div>

    <p v-if="!roots.length && !loading" class="admin-muted mt-4">
      Пока никто никого не приглашал. Дерево появится, когда по выписанному коду зарегистрируются.
    </p>

    <SamsungModal v-model="showCut" title="Срезать ветвь">
      <p class="body-copy">
        Уйдёт <span class="wordmark-inline">{{ pending?.username }}</span> и все, кого он привёл. Причина попадёт в
        аудит.
      </p>
      <OneuiInput v-model.trim="reason" label="Причина" placeholder="продажа аккаунтов" class="mt-4" />
      <template #footer>
        <SamsungButton variant="ghost" @click="showCut = false">Отмена</SamsungButton>
        <SamsungButton :busy="busyID === pending?.admin_id" @click="confirmCut">Срезать</SamsungButton>
      </template>
    </SamsungModal>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import InviteTreeNode from '@/components/domain/InviteTreeNode.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungModal from '@/components/layout/SamsungModal.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import { formatBytes } from '@/utils/format';

const members = ref([]);
// Раскрытые ветви. Пусто по умолчанию: дерево на полсотни человек, развёрнутое
// целиком, читается не лучше исходной свалки.
const opened = ref({});

function toggle(adminID) {
  opened.value = { ...opened.value, [adminID]: !opened.value[adminID] };
}
const loading = ref(false);
const loadError = ref('');
const busyID = ref(0);
const showCut = ref(false);
const pending = ref(null);
const reason = ref('');

// Depth-first, so a branch reads as a branch rather than as a flat list sorted
// by whatever the database felt like returning.
// В дереве только те, кто кого-то пригласил или сам пришёл по приглашению.
// Администраторы, заведённые до дерева, к приглашениям отношения не имеют и в
// нём не показываются - иначе список выглядит так, будто доступ раздавали всем
// подряд.
const branches = computed(() => {
  const hasChildren = new Set(members.value.map((m) => m.invited_by).filter(Boolean));
  return ordered.value.filter((m) => m.invited_by || hasChildren.has(m.admin_id));
});

// Дерево рисуется от корней вниз рекурсивным узлом, поэтому здесь только те,
// у кого нет пригласившего
const roots = computed(() => branches.value.filter((m) => !m.invited_by));

const ordered = computed(() => {
  const byParent = new Map();
  for (const m of members.value) {
    const key = m.invited_by || 0;
    if (!byParent.has(key)) byParent.set(key, []);
    byParent.get(key).push(m);
  }
  for (const list of byParent.values()) {
    list.sort((a, b) => a.username.localeCompare(b.username));
  }
  const out = [];
  const walk = (parent, seen) => {
    for (const m of byParent.get(parent) || []) {
      if (seen.has(m.admin_id)) continue;
      seen.add(m.admin_id);
      out.push(m);
      walk(m.admin_id, seen);
    }
  };
  walk(0, new Set());
  return out;
});

onMounted(load);

async function load() {
  loading.value = true;
  try {
    const res = await fetch('/api/owner/invite-tree', { credentials: 'include' });
    if (!res.ok) throw new Error(await errorText(res));
    members.value = (await res.json()).members || [];
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    loading.value = false;
  }
}

function cut(member) {
  pending.value = member;
  reason.value = '';
  showCut.value = true;
}

async function confirmCut() {
  if (!pending.value) return;
  await act(pending.value, 'cut', { reason: reason.value });
  showCut.value = false;
  pending.value = null;
}

async function restore(member) {
  await act(member, 'restore', {});
}

async function act(member, action, body) {
  busyID.value = member.admin_id;
  try {
    const res = await fetch(`/api/owner/invite-tree/${member.admin_id}/${action}`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(await errorText(res));
    await load();
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    busyID.value = 0;
  }
}

function bytes(value) {
  return formatBytes(value || 0);
}

async function errorText(res) {
  try {
    return (await res.json()).error || `HTTP ${res.status}`;
  } catch {
    return `HTTP ${res.status}`;
  }
}
</script>
