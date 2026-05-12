<template>
  <div class="client-page">
    <SamsungCard kicker="Master" title="Общие настройки клиентов">
      <p class="body-copy">
        Здесь хранится конфигурация, которую можно массово раскатать на всех ваших клиентов одной кнопкой.
        Уникальные поля (Xray-профили и подписки, ключи WireGuard, комната WB Stream, аватар, sharing-настройки)
        никогда не перезаписываются — обновятся только секции, отмеченные ниже.
      </p>
      <p v-if="loadError" class="admin-error mt-3">{{ loadError }}</p>
      <SamsungSectionLoader v-else-if="loading" />
    </SamsungCard>

    <SamsungCard v-if="!loading && !loadError" title="Заполнить из источника">
      <p class="body-copy">
        Можно загрузить шаблон с уже настроенного клиента или из
        <code class="font-sharp">wingsv://</code> / <code class="font-sharp">vless://</code>
        ссылки — уникальные поля (Xray-профили и подписки, ключи WG, комната WB Stream, sharing/xposed)
        автоматически выбрасываются.
      </p>
      <div class="mt-3">
        <OneuiRadioGroup v-model="seedMode" :options="seedModeOptions" variant="pill" />
      </div>
      <template v-if="seedMode === 'clone'">
        <label class="field-label mt-4">Источник</label>
        <OneuiSelect
          :model-value="seedFromClientId"
          :options="seedClientOptions"
          @update:model-value="seedFromClientId = $event"
        />
      </template>
      <template v-else-if="seedMode === 'link'">
        <OneuiTextarea
          v-model.trim="seedLink"
          label="Ссылка (wingsv:// или vless://)"
          rows="3"
          placeholder="wingsv://... или vless://..."
        />
      </template>
      <p v-if="seedError" class="admin-error mt-3">{{ seedError }}</p>
      <p v-if="seedNote" class="admin-muted mt-3">{{ seedNote }}</p>
      <div v-if="seedMode !== 'empty'" class="actions-row mt-4">
        <SamsungButton :busy="busySeed" :disabled="!canSeed" @click="onSeed">
          <template #icon><DownloadCloud class="button-icon" aria-hidden="true" /></template>
          {{ busySeed ? "Загружаем…" : "Загрузить шаблон" }}
        </SamsungButton>
      </div>
    </SamsungCard>

    <SamsungCard v-if="!loading && !loadError" title="Что переносим">
      <p class="body-copy">Включите тумблер, чтобы массовое применение трогало эту секцию.</p>
      <div class="master-scope-grid mt-4">
        <label v-for="opt in scopeOptions" :key="opt.id" class="master-scope-row">
          <div>
            <div class="master-scope-label">{{ opt.label }}</div>
            <div class="master-scope-hint">{{ opt.hint }}</div>
          </div>
          <OneuiSwitch :model-value="scopeFlags[opt.id]" @change="scopeFlags[opt.id] = $event" />
        </label>
      </div>
    </SamsungCard>

    <SamsungCard v-if="!loading && !loadError && scopeFlags.sync" title="Синхронизация">
      <p class="body-copy">Режим, в котором клиенты получают изменения из панели.</p>
      <div class="mt-3">
        <OneuiRadioGroup v-model="syncMode" :options="syncModeOptions" variant="pill" />
      </div>
      <div v-if="syncMode === 'periodic'" class="form-row form-row-stack mt-3">
        <OneuiInput
          v-model.number="syncIntervalMinutes"
          label="Интервал (мин, минимум 15)"
          type="number"
          :min="15"
          narrow
        />
      </div>
    </SamsungCard>

    <SamsungCard v-if="!loading && !loadError" title="Шаблон конфигурации">
      <p class="body-copy">
        Заполните только те секции, которые включены выше. Остальные значения тут можно оставить пустыми —
        панель не тронет их при применении.
      </p>
      <ConfigFormEditor
        :model-value="formValue"
        :sections="visibleSections"
        @update:model-value="onFormChanged"
      />
    </SamsungCard>

    <SamsungCard v-if="!loading && !loadError" title="Применение">
      <p class="body-copy">
        После сохранения нажмите «Применить ко всем», чтобы разлить включённые секции по всем вашим клиентам.
        Подключённые клиенты получат обновление мгновенно; остальные — при следующей синхронизации.
      </p>
      <p v-if="saveError" class="admin-error mt-3">{{ saveError }}</p>
      <p v-if="applyError" class="admin-error mt-3">{{ applyError }}</p>
      <p v-if="applyResult" class="admin-muted mt-3">
        Применено: пушнули {{ applyResult.clients_pushed }} из {{ applyResult.clients_total }} клиентов.
      </p>
      <p v-if="lastSavedAt" class="admin-muted mt-2">Сохранено: {{ formatTs(lastSavedAt) }}</p>
      <div class="actions-row mt-4">
        <SamsungButton :busy="busySave" @click="onSave">
          <template #icon><Save class="button-icon" aria-hidden="true" /></template>
          {{ busySave ? "Сохраняем…" : "Сохранить" }}
        </SamsungButton>
        <SamsungButton variant="secondary" :busy="busyApply" :disabled="!hasAnyScope" @click="askApply">
          <template #icon><UploadCloud class="button-icon" aria-hidden="true" /></template>
          {{ busyApply ? "Раскатываем…" : "Применить ко всем" }}
        </SamsungButton>
      </div>
    </SamsungCard>

    <SamsungModal v-model="confirmApply" title="Применить ко всем клиентам?" :busy="busyApply">
      <p class="body-copy">
        Будут обновлены секции:
        <strong>{{ enabledScopeLabels.join(", ") || "—" }}</strong>.
        Остальная конфигурация клиентов сохранится без изменений.
      </p>
      <template #actions>
        <SamsungButton :busy="busyApply" @click="performApply">
          <template #icon><UploadCloud class="button-icon" aria-hidden="true" /></template>
          {{ busyApply ? "Раскатываем…" : "Применить" }}
        </SamsungButton>
        <SamsungButton variant="secondary" :disabled="busyApply" @click="cancelApply">
          <template #icon><X class="button-icon" aria-hidden="true" /></template>
          Отмена
        </SamsungButton>
      </template>
    </SamsungModal>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from "vue";
import { DownloadCloud, Save, UploadCloud, X } from "lucide-vue-next";
import OneuiInput from "@/components/controls/OneuiInput.vue";
import OneuiRadioGroup from "@/components/controls/OneuiRadioGroup.vue";
import OneuiSelect from "@/components/controls/OneuiSelect.vue";
import OneuiSwitch from "@/components/controls/OneuiSwitch.vue";
import OneuiTextarea from "@/components/controls/OneuiTextarea.vue";
import SamsungButton from "@/components/layout/SamsungButton.vue";
import SamsungCard from "@/components/layout/SamsungCard.vue";
import SamsungModal from "@/components/layout/SamsungModal.vue";
import SamsungSectionLoader from "@/components/layout/SamsungSectionLoader.vue";
import ConfigFormEditor from "@/components/domain/ConfigFormEditor.vue";

// Backend scope tokens (must match `internal/handlers/admin/master.go`).
// Each token enables a single top-level Config section in the bulk apply.
const scopeOptions = [
  { id: "sync", label: "Режим синхронизации", hint: "always / periodic / foreground + интервал" },
  { id: "turn", label: "VK TURN", hint: "endpoint, ссылки, threads, обфускация" },
  { id: "xray_settings", label: "Xray (общие)", hint: "DNS, sniffing, mux — без профилей и подписок" },
  { id: "xray_routing", label: "Xray routing", hint: "Правила маршрутизации (домены, гео, IP)" },
  { id: "byedpi", label: "ByeDPI", hint: "Профиль обхода DPI" },
  { id: "app_preferences", label: "Приложение", hint: "Тема, DNS, автозапуск" },
  { id: "app_routing", label: "Per-app routing", hint: "Список приложений в туннеле" },
];

const scopeFlags = reactive({
  sync: false,
  turn: false,
  xray_settings: false,
  xray_routing: false,
  byedpi: false,
  app_preferences: false,
  app_routing: false,
});

const enabledScopeLabels = computed(() =>
  scopeOptions.filter((o) => scopeFlags[o.id]).map((o) => o.label),
);
const hasAnyScope = computed(() => Object.values(scopeFlags).some(Boolean));

// Map scope flag → editor section id (ConfigFormEditor uses different ids).
const scopeToFormSection = {
  turn: "vk_turn",
  xray_settings: "xray",
  byedpi: "byedpi",
  app_preferences: "app",
  app_routing: "app_routing",
};

const visibleSections = computed(() => {
  const ids = new Set();
  for (const [scope, section] of Object.entries(scopeToFormSection)) {
    if (scopeFlags[scope]) ids.add(section);
  }
  return Array.from(ids);
});

const syncMode = ref("always");
const syncIntervalMinutes = ref(30);
const configDraft = ref("{}");
const lastSavedAt = ref(null);

const loading = ref(false);
const loadError = ref("");
const busySave = ref(false);
const saveError = ref("");
const busyApply = ref(false);
const applyError = ref("");
const applyResult = ref(null);
const confirmApply = ref(false);

const clients = ref([]);
const seedMode = ref("empty");
const seedFromClientId = ref("");
const seedLink = ref("");
const busySeed = ref(false);
const seedError = ref("");
const seedNote = ref("");

const canSeed = computed(() => {
  if (seedMode.value === "clone") return !!seedFromClientId.value;
  if (seedMode.value === "link") return !!seedLink.value;
  return false;
});

const seedClientOptions = computed(() => [
  { value: "", label: "— выберите клиента —" },
  ...clients.value.map((c) => ({ value: c.id, label: `${c.name} · ${c.id}` })),
]);

const seedModeOptions = computed(() => [
  { value: "empty", label: "Пусто (вручную)" },
  { value: "clone", label: "С клиента", disabled: clients.value.length === 0 },
  { value: "link", label: "Из wingsv:// / vless://" },
]);

const syncModeOptions = [
  { value: "always", label: "Всегда в фоне" },
  { value: "periodic", label: "Периодически" },
  { value: "foreground", label: "Только когда приложение открыто" },
];

const formValue = computed({
  get() {
    try {
      return JSON.parse(configDraft.value || "{}");
    } catch (err) {
      return {};
    }
  },
  set(next) {
    configDraft.value = JSON.stringify(next || {}, null, 2);
  },
});

function onFormChanged(next) {
  configDraft.value = JSON.stringify(next || {}, null, 2);
}

async function loadClientsList() {
  try {
    const res = await fetch("/api/admin/clients", { credentials: "include" });
    if (!res.ok) return;
    const data = await res.json();
    clients.value = data.clients || [];
  } catch (err) {
    // Non-fatal: seed-from-client is just disabled.
  }
}

// Auto-enable scope flags for sections that the seeded config actually carries.
function enableScopesFromConfig(cfg) {
  if (!cfg || typeof cfg !== "object") return;
  if (cfg.turn) scopeFlags.turn = true;
  if (cfg.xray && (cfg.xray.settings || cfg.xray.allowLan != null || cfg.xray.remoteDns)) {
    scopeFlags.xray_settings = true;
  }
  if (cfg.xray && cfg.xray.routing) scopeFlags.xray_routing = true;
  if (cfg.byeDpi || cfg.bye_dpi) scopeFlags.byedpi = true;
  if (cfg.appPreferences || cfg.app_preferences) scopeFlags.app_preferences = true;
  if (cfg.appRouting || cfg.app_routing) scopeFlags.app_routing = true;
}

async function onSeed() {
  if (busySeed.value || !canSeed.value) return;
  busySeed.value = true;
  seedError.value = "";
  seedNote.value = "";
  try {
    const body = {};
    if (seedMode.value === "clone") body.from_client_id = seedFromClientId.value;
    else if (seedMode.value === "link") body.from_wingsv_link = seedLink.value;
    const res = await fetch("/api/admin/master-config/seed", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}));
      throw new Error(errBody.message || "Не удалось загрузить шаблон");
    }
    const data = await res.json();
    const cfg = data.config || {};
    configDraft.value = JSON.stringify(cfg, null, 2);
    enableScopesFromConfig(cfg);
    seedNote.value = "Шаблон загружен. Проверьте секции и нажмите «Сохранить».";
  } catch (err) {
    seedError.value = err.message || "Не удалось загрузить шаблон";
  } finally {
    busySeed.value = false;
  }
}

async function load() {
  loading.value = true;
  loadError.value = "";
  try {
    const res = await fetch("/api/admin/master-config", { credentials: "include" });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    const flags = Array.isArray(data.scope_flags) ? data.scope_flags : [];
    for (const k of Object.keys(scopeFlags)) {
      scopeFlags[k] = flags.includes(k);
    }
    syncMode.value = data.sync_mode || "always";
    syncIntervalMinutes.value = Number(data.periodic_interval_minutes) || 30;
    if (data.config && data.config !== null && data.config !== "null") {
      configDraft.value = JSON.stringify(data.config, null, 2);
    } else {
      configDraft.value = "{}";
    }
    lastSavedAt.value = data.updated_at || null;
  } catch (err) {
    loadError.value = err.message || "Не удалось загрузить master-настройки";
  } finally {
    loading.value = false;
  }
}

async function onSave() {
  if (busySave.value) return;
  busySave.value = true;
  saveError.value = "";
  applyResult.value = null;
  try {
    let parsed = {};
    try {
      parsed = JSON.parse(configDraft.value || "{}");
    } catch (err) {
      throw new Error("Некорректный JSON в конфигурации");
    }
    const flags = Object.keys(scopeFlags).filter((k) => scopeFlags[k]);
    const body = {
      config: parsed,
      sync_mode: scopeFlags.sync ? syncMode.value : "",
      periodic_interval_minutes: scopeFlags.sync && syncMode.value === "periodic"
        ? Math.max(15, Number(syncIntervalMinutes.value) || 30)
        : 0,
      scope_flags: flags,
    };
    const res = await fetch("/api/admin/master-config", {
      method: "PUT",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}));
      throw new Error(errBody.message || "Не удалось сохранить");
    }
    await load();
  } catch (err) {
    saveError.value = err.message || "Не удалось сохранить";
  } finally {
    busySave.value = false;
  }
}

function askApply() {
  if (!hasAnyScope.value) return;
  applyError.value = "";
  applyResult.value = null;
  confirmApply.value = true;
}

function cancelApply() {
  if (busyApply.value) return;
  confirmApply.value = false;
}

async function performApply() {
  if (busyApply.value) return;
  busyApply.value = true;
  applyError.value = "";
  applyResult.value = null;
  try {
    const res = await fetch("/api/admin/master-config/apply", {
      method: "POST",
      credentials: "include",
    });
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}));
      throw new Error(errBody.message || "Не удалось применить");
    }
    applyResult.value = await res.json();
    confirmApply.value = false;
  } catch (err) {
    applyError.value = err.message || "Не удалось применить";
  } finally {
    busyApply.value = false;
  }
}

function formatTs(iso) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("ru-RU");
  } catch (err) {
    return iso;
  }
}

onMounted(() => {
  load();
  loadClientsList();
});
</script>
