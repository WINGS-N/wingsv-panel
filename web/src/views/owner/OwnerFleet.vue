<template>
  <section class="surface-card">
    <div class="federation-live-head">
      <h2 class="section-title">Флот</h2>
      <span v-if="fleet.config_version" class="admin-pill is-info">конфиг {{ fleet.config_version }}</span>
    </div>
    <p class="body-copy body-copy-wide">
      Какие сборки несут все узлы сразу. Версия выбирается из релизов нашего форка - апстримовый Xray не несёт патчей
      WINGS, и узел с ним выглядит здоровым, пока статистика по пирам остаётся пустой.
    </p>
    <p v-if="loadError" class="state-error mt-3">{{ loadError }}</p>

    <div class="form-grid mt-5">
      <label class="input-field">
        <span class="field-label">Сборка Xray</span>
        <select v-model="selectedXray" class="fleet-select">
          <option value="">не менять</option>
          <option v-for="r in xrayReleases" :key="r.tag" :value="r.tag" :disabled="!r.asset_url">
            {{ r.tag }}{{ r.prerelease ? ' (pre)' : '' }}{{ r.asset_url ? '' : ' - нет сборки под linux' }}
          </option>
        </select>
      </label>
      <OneuiInput v-model.trim="fleet.reality_dest" label="REALITY dest" placeholder="www.microsoft.com:443" />
      <OneuiInput v-model.number="fleet.tcp_port" label="Порт TCP" type="number" :min="1" :max="65535" />
      <OneuiInput v-model.number="fleet.xhttp_port" label="Порт XHTTP" type="number" :min="1" :max="65535" />
    </div>

    <div class="fleet-toggles mt-4">
      <label class="fleet-toggle">
        <OneuiSwitch v-model="fleet.auto_upgrade" />
        <span>
          <span class="fleet-toggle-name">Обновляться самим</span>
          <span class="fleet-toggle-hint">
            Узел поставит новую сборку сразу. Перезапуск рвёт живые соединения, включая клиентов самого донора.
          </span>
        </span>
      </label>
      <label class="fleet-toggle">
        <OneuiSwitch v-model="fleet.post_quantum" />
        <span>
          <span class="fleet-toggle-name">ML-DSA-65 поверх REALITY</span>
          <span class="fleet-toggle-hint"
            >Помещается не в каждый заимствованный хендшейк - проверяйте после смены dest.</span
          >
        </span>
      </label>
      <label class="fleet-toggle">
        <OneuiSwitch v-model="fleet.auto_dest" />
        <span>
          <span class="fleet-toggle-name">Выбирать dest автоматически</span>
          <span class="fleet-toggle-hint">Голова берёт проверенный из пула, если поле выше пустое.</span>
        </span>
      </label>
    </div>

    <div class="actions-row">
      <SamsungButton :busy="saving" @click="save">
        <template #icon><Save class="button-icon" aria-hidden="true" /></template>
        Применить ко флоту
      </SamsungButton>
      <SamsungButton variant="ghost" :busy="restarting" @click="restart('xray')">
        <template #icon><RefreshCw class="button-icon" aria-hidden="true" /></template>
        Перезапустить Xray везде
      </SamsungButton>
    </div>
    <p v-if="notice" class="admin-muted mt-3 text-[13px]">{{ notice }}</p>
  </section>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue';
import { RefreshCw, Save } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import OneuiSwitch from '@/components/controls/OneuiSwitch.vue';

const fleet = reactive({
  xray_version: '',
  xray_url: '',
  reality_dest: '',
  auto_dest: false,
  post_quantum: false,
  auto_upgrade: false,
  tcp_port: 443,
  xhttp_port: 8443,
  config_version: 0,
});
const xrayReleases = ref([]);
const selectedXray = ref('');
const loadError = ref('');
const notice = ref('');
const saving = ref(false);
const restarting = ref(false);

onMounted(async () => {
  await Promise.all([load(), loadReleases()]);
});

async function load() {
  try {
    const res = await fetch('/api/admin/fleet', { credentials: 'include' });
    if (!res.ok) throw new Error(await errorText(res));
    Object.assign(fleet, await res.json());
    selectedXray.value = fleet.xray_version || '';
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  }
}

async function loadReleases() {
  try {
    const res = await fetch('/api/admin/fleet/releases', { credentials: 'include' });
    if (!res.ok) return;
    xrayReleases.value = (await res.json()).releases || [];
  } catch {
    // Список релизов - удобство: без него настройки всё равно сохраняются
  }
}

async function save() {
  saving.value = true;
  try {
    // Пустой выбор означает "не трогать сборку": иначе сохранение любой другой
    // настройки снесло бы флоту Xray
    const chosen = xrayReleases.value.find((r) => r.tag === selectedXray.value);
    const body = {
      ...fleet,
      xray_version: chosen ? chosen.tag : '',
      xray_url: chosen ? chosen.asset_url : '',
    };
    const res = await fetch('/api/admin/fleet', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(await errorText(res));
    Object.assign(fleet, await res.json());
    loadError.value = '';
    notice.value = `Применено. Узлы подхватят конфиг ${fleet.config_version} на ближайшем подключении.`;
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    saving.value = false;
  }
}

async function restart(component) {
  restarting.value = true;
  try {
    const res = await fetch('/api/admin/fleet/restart', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ component }),
    });
    if (!res.ok) throw new Error(await errorText(res));
    notice.value = `Команда ушла на ${(await res.json()).nodes} узлов.`;
    loadError.value = '';
  } catch (err) {
    loadError.value = String(err.message || err);
  } finally {
    restarting.value = false;
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
