<template>
  <div class="client-page">
    <section class="surface-card">
      <header class="admin-card-header admin-detail-header">
        <div class="flex items-center gap-4">
          <span class="client-avatar client-avatar-lg">
            <SamsungLoader v-if="!detail" />
            <template v-else>{{ headerInitials }}</template>
          </span>
          <div>
            <router-link class="admin-back-link" :to="{ name: 'admin-clients' }">← Все клиенты</router-link>
            <h1 class="admin-card-title">{{ detail?.client?.name || id }}</h1>
            <div
              v-if="detail"
              class="flex flex-wrap items-center gap-x-3 gap-y-1 text-[13px] text-wings-muted"
            >
              <SamsungPill :variant="detail.client?.online ? 'online' : 'offline'">
                {{ detail.client?.online ? "Онлайн" : "Оффлайн" }}
              </SamsungPill>
              <span v-if="detail.client?.backend_type">{{ detail.client.backend_type }}</span>
              <span>·</span>
              <code class="admin-mono">{{ id }}</code>
              <span>·</span>
              <span>{{ detail.client?.device_model || "—" }}</span>
              <span>·</span>
              <span>WINGS V {{ detail.client?.app_version || "—" }}</span>
              <span v-if="detail.client?.created_at">·</span>
              <span v-if="detail.client?.created_at">Добавлен {{ formatDate(detail.client.created_at) }}</span>
            </div>
          </div>
        </div>
        <div class="admin-detail-actions">
          <SamsungButton variant="secondary" :busy="busyLink" @click="showLink">
            <template #icon><Link2 class="button-icon" aria-hidden="true" /></template>
            Показать ссылку
          </SamsungButton>
        </div>
      </header>

      <SamsungModal
        :model-value="showLinkModal && !!wingsvLink"
        title="Ссылка на клиент"
        @update:model-value="dismissLink"
      >
        <p class="body-copy">Откройте её в WINGS V на устройстве клиента или отсканируйте QR-код.</p>
        <div class="qr-frame">
          <img v-if="wingsvLinkQR" :src="wingsvLinkQR" alt="QR" />
        </div>
        <CopyableLink :value="wingsvLink" rows="3" />
        <template #actions>
          <SamsungButton @click="dismissLink">
            <template #icon><X class="button-icon" aria-hidden="true" /></template>
            Закрыть
          </SamsungButton>
        </template>
      </SamsungModal>

      <p v-if="loadError" class="admin-error">{{ loadError }}</p>

      <div class="admin-tabs">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          :class="['admin-tab', activeTab === tab.id ? 'is-active' : '']"
          @click="setActiveTab(tab.id)"
        >{{ tab.label }}</button>
      </div>
    </section>

    <section v-if="activeTab === 'config'" class="client-grid">
      <div class="surface-card">
        <div class="text-[12px] font-bold uppercase tracking-[0.14em] text-wings-kicker">Подключение</div>
        <h3 class="font-sharp text-[22px] font-bold mt-2">{{ connectionTitle }}</h3>
        <dl class="keyvals mt-6">
          <template v-for="row in connectionFacts" :key="row.label">
            <dt>{{ row.label }}</dt>
            <dd :class="row.mono ? 'mono' : ''">{{ row.value || "—" }}</dd>
          </template>
        </dl>
        <div class="actions-row mt-6">
          <SamsungButton @click="copyLinkInline">
            <template #icon><Copy class="button-icon" aria-hidden="true" /></template>
            {{ copiedLink ? "Скопировано" : "Скопировать ссылку" }}
          </SamsungButton>
          <SamsungButton variant="secondary" :busy="busyRotate" @click="onRotateToken">
            <template #icon><RefreshCw class="button-icon" aria-hidden="true" /></template>
            {{ busyRotate ? "Ротируем…" : "Ротировать токен" }}
          </SamsungButton>
        </div>
      </div>

      <div class="surface-card">
        <div class="text-[12px] font-bold uppercase tracking-[0.14em] text-wings-kicker">Поделиться</div>
        <div class="qr-block">
          <div class="qr-frame">
            <img v-if="wingsvLinkQR" :src="wingsvLinkQR" alt="QR-код wingsv-ссылки" />
            <div v-else class="qr-placeholder">
              <div class="samsung-loader">
                <span class="samsung-loader-dot samsung-loader-dot-top"></span>
                <span class="samsung-loader-dot samsung-loader-dot-right"></span>
                <span class="samsung-loader-dot samsung-loader-dot-bottom"></span>
                <span class="samsung-loader-dot samsung-loader-dot-left"></span>
              </div>
            </div>
          </div>
          <div class="font-bold admin-mono text-center">{{ wingsvLinkPreview }}</div>
        </div>
      </div>
    </section>

    <section v-if="activeTab === 'config'" class="surface-card">

      <div class="config-mode-tabs">
        <button :class="['admin-tab', configMode === 'form' ? 'is-active' : '']" @click="setConfigMode('form')">Форма</button>
        <button :class="['admin-tab', configMode === 'json' ? 'is-active' : '']" @click="setConfigMode('json')">JSON</button>
        <span class="admin-follow-toggle">
          <OneuiSwitch :model-value="followClient" @change="setFollowClient($event)" />
          <span class="admin-follow-label">Следить за клиентом</span>
        </span>
      </div>
      <div class="actions-row mt-3">
        <SamsungButton variant="secondary" :disabled="!detail?.reported_config" @click="loadFromReported">
          <template #icon><Download class="button-icon" aria-hidden="true" /></template>
          Загрузить текущий с клиента
        </SamsungButton>
      </div>
      <details class="admin-config-import">
        <summary>Импорт из wingsv:// или vless:// ссылки</summary>
        <p class="admin-muted">
          Вставьте ссылку <code class="font-sharp">wingsv://</code> или <code class="font-sharp">vless://</code>
          — её содержимое заменит текущий черновик. Изменения не отправятся клиенту, пока вы не нажмёте «Применить».
        </p>
        <textarea
          v-model.trim="importLinkDraft"
          class="text-input admin-import-area"
          rows="3"
          spellcheck="false"
          placeholder="wingsv://... или vless://..."
        />
        <p v-if="importError" class="admin-error">{{ importError }}</p>
        <div class="actions-row mt-2">
          <SamsungButton variant="secondary" :busy="busyImport" :disabled="!importLinkDraft" @click="importFromLink">
            <template #icon><Link2 class="button-icon" aria-hidden="true" /></template>
            {{ busyImport ? "Распаковываем…" : "Подставить" }}
          </SamsungButton>
        </div>
      </details>
      <ConfigFormEditor
        v-if="configMode === 'form'"
        :model-value="formValue"
        :has-root-access="!!detail.client?.has_root_access"
        @update:model-value="onFormChanged"
      />
      <JsonEditor v-else v-model="configDraft" height="fixed" />
      <p v-if="configError" class="admin-error">{{ configError }}</p>
      <div class="actions-row mt-4">
        <SamsungButton :busy="busyPush" @click="pushConfig">
          <template #icon><UploadCloud class="button-icon" aria-hidden="true" /></template>
          {{ busyPush ? "Отправляем…" : "Применить (Push)" }}
        </SamsungButton>
        <SamsungButton variant="secondary" @click="resetConfigDraft">
          <template #icon><RotateCcw class="button-icon" aria-hidden="true" /></template>
          Сбросить правки
        </SamsungButton>
      </div>
    </section>

    <div v-if="activeTab === 'xray_rules'" class="surface-card admin-tab-pane">
      <div class="admin-sticky-bar">
        <p class="admin-muted">Правила маршрутизации Xray (порядок имеет значение — первое совпадение применяется).</p>
        <SamsungButton :busy="busyPush" @click="pushConfig">
          <template #icon><UploadCloud class="button-icon" aria-hidden="true" /></template>
          {{ busyPush ? "Отправляем…" : "Применить (Push)" }}
        </SamsungButton>
      </div>
      <div class="form-section mt-3">
        <div class="form-row form-row-stack">
          <label class="form-label">GeoIP URL</label>
          <input class="text-input" :value="xrayRouting.geoipUrl || ''" @input="setXrayRouting('geoipUrl', $event.target.value)" placeholder="https://...geoip.dat" />
        </div>
        <div class="form-row form-row-stack">
          <label class="form-label">GeoSite URL</label>
          <input class="text-input" :value="xrayRouting.geositeUrl || ''" @input="setXrayRouting('geositeUrl', $event.target.value)" placeholder="https://...geosite.dat" />
        </div>
      </div>
      <div class="actions-row mt-3">
        <SamsungButton variant="secondary" @click="addRule">
          <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
          Добавить правило
        </SamsungButton>
      </div>
      <ul v-if="xrayRules.length" class="admin-list mt-3">
        <li
          v-for="(rule, idx) in xrayRules"
          :key="rule.id || idx"
          :class="['admin-rule-card', dragOverIdx === idx ? 'is-drag-over' : '', draggingIdx === idx ? 'is-dragging' : '']"
          draggable="true"
          @dragstart="onRuleDragStart(idx, $event)"
          @dragover.prevent="onRuleDragOver(idx, $event)"
          @dragleave="dragOverIdx = -1"
          @drop.prevent="onRuleDrop(idx)"
          @dragend="onRuleDragEnd"
        >
          <div class="admin-rule-handle" aria-hidden="true">
            <GripVertical class="button-icon" />
          </div>
          <div class="admin-rule-body">
            <div class="form-row">
              <label class="form-label">Тип</label>
              <OneuiSelect :model-value="rule.matchType || 'XRAY_ROUTING_MATCH_UNSPECIFIED'" :options="matchTypeOptions" @change="patchRule(idx, 'matchType', $event)" />
            </div>
            <div class="form-row form-row-stack">
              <label class="form-label">{{ matchCodeLabel(rule.matchType) }}</label>
              <input class="text-input" :value="rule.code || ''" @input="patchRule(idx, 'code', $event.target.value)" :placeholder="matchCodePlaceholder(rule.matchType)" />
            </div>
            <div class="form-row">
              <label class="form-label">Действие</label>
              <OneuiSelect :model-value="rule.action || 'XRAY_ROUTING_ACTION_UNSPECIFIED'" :options="actionOptions" @change="patchRule(idx, 'action', $event)" />
            </div>
            <div class="form-row">
              <label class="form-label">Включено</label>
              <OneuiSwitch :model-value="rule.enabled !== false" @change="patchRule(idx, 'enabled', $event)" />
            </div>
          </div>
          <div class="admin-rule-actions">
            <SamsungIconButton size="small" :title="'Удалить'" @click="removeRule(idx)">
              <Trash2 class="button-icon" aria-hidden="true" />
            </SamsungIconButton>
          </div>
        </li>
      </ul>
      <p v-else class="admin-muted mt-3">Правил нет.</p>
    </div>

    <div v-if="activeTab === 'app_routing'" class="surface-card admin-tab-pane">
      <div class="admin-sticky-bar">
        <p class="admin-muted">Per-app routing: какие приложения идут через тунель.</p>
        <SamsungButton :busy="busyPush" @click="pushConfig">
          <template #icon><UploadCloud class="button-icon" aria-hidden="true" /></template>
          {{ busyPush ? "Отправляем…" : "Применить (Push)" }}
        </SamsungButton>
      </div>
      <div class="form-section mt-3">
        <div class="form-row">
          <label class="form-label">Режим bypass (исключать вместо включать)</label>
          <OneuiSwitch :model-value="!!appRouting.bypass" @change="setAppRoutingField('bypass', $event)" />
        </div>
      </div>
      <div class="form-section mt-3">
        <div class="form-row">
          <h3 class="form-section-title m-0">Установленные приложения</h3>
          <SamsungButton variant="secondary" :busy="busyAppsRefresh" @click="refreshInstalledApps">
            <template #icon><RotateCw class="button-icon" aria-hidden="true" /></template>
            Обновить список
          </SamsungButton>
        </div>
        <p v-if="installedAppsUpdated" class="admin-muted">Получено: {{ formatTs(installedAppsUpdated) }}</p>
        <p v-else class="admin-muted">Список ещё не получен с устройства. Нажмите «Обновить список» — клиент должен быть онлайн.</p>
        <div v-if="installedApps.length" class="mt-3">
          <OneuiRadioGroup
            v-model="appsKindFilter"
            :options="appsKindFilterOptions"
            variant="pill"
          />
        </div>
        <div v-if="installedApps.length" class="form-row form-row-stack mt-3">
          <input class="text-input" v-model.trim="appsFilter" placeholder="Фильтр по имени или пакету" />
        </div>
        <ul v-if="filteredInstalledApps.length" class="admin-list mt-3 admin-apps-grid">
          <li
            v-for="app in filteredInstalledApps"
            :key="app.package"
            :class="['admin-apps-item', isPackageRouted(app.package) ? 'is-routed' : '']"
          >
            <img v-if="app.icon" :src="app.icon" alt="" class="admin-apps-icon" />
            <div v-else class="admin-apps-icon admin-apps-icon-fallback" aria-hidden="true">
              {{ (app.label || app.package || "?").slice(0, 1).toUpperCase() }}
            </div>
            <div class="admin-apps-text">
              <strong>{{ app.label || app.package }}</strong>
              <span class="admin-muted">{{ app.package }}</span>
              <div class="mt-1 flex flex-wrap gap-1">
                <SamsungPill v-if="app.recommended" variant="online">Рекомендуется</SamsungPill>
                <SamsungPill v-if="app.system" variant="offline">Системное</SamsungPill>
              </div>
            </div>
            <OneuiSwitch
              :model-value="isPackageRouted(app.package)"
              @change="togglePackageRouted(app.package, $event)"
            />
          </li>
        </ul>
      </div>
      <details class="admin-config-import mt-3">
        <summary>Редактировать список текстом</summary>
        <textarea
          class="text-input mt-2"
          rows="6"
          spellcheck="false"
          :value="(appRouting.packages || []).join('\n')"
          @input="setAppRoutingPackages($event.target.value)"
          placeholder="com.example.app"
        ></textarea>
      </details>
    </div>

    <div v-if="activeTab === 'state'" class="surface-card admin-tab-pane">
      <div class="form-section">
        <h3 class="form-section-title">Режим синхронизации</h3>
        <p class="admin-muted">Если клиент онлайн — изменения применяются мгновенно. Иначе подхватятся при следующем коннекте.</p>
        <div class="admin-pill-row mt-2">
          <button
            v-for="opt in syncModeOptions"
            :key="opt.id"
            type="button"
            :class="['admin-pill-button', syncModeDraft === opt.id ? 'is-active' : '']"
            @click="setSyncMode(opt.id)"
          >
            <component :is="opt.icon" class="button-icon" aria-hidden="true" />
            <span>{{ opt.label }}</span>
          </button>
        </div>
        <div v-if="syncModeDraft === 'periodic'" class="form-row form-row-stack mt-3">
          <label class="form-label">Интервал (мин, минимум 15)</label>
          <input
            v-model.number="syncIntervalDraft"
            class="text-input form-input-narrow"
            type="number"
            min="15"
            @change="saveSyncSettings"
          />
        </div>
      </div>

      <h2 class="admin-section-subtitle mt-5">Runtime</h2>
      <pre class="admin-code">{{ formatJson(detail?.runtime) }}</pre>
      <p class="admin-muted">Обновлено: {{ formatTs(detail?.runtime_updated) }}</p>
      <h2 class="admin-section-subtitle mt-5">Что репортит клиент</h2>
      <pre class="admin-code">{{ formatJson(detail?.reported_config) }}</pre>
      <p class="admin-muted">Обновлено: {{ formatTs(detail?.reported_config_updated) }}</p>
    </div>

    <div v-if="activeTab === 'logs'" class="surface-card admin-tab-pane">
      <div class="admin-log-controls">
        <OneuiCheckbox
          v-for="stream in logStreams"
          :key="stream.id"
          :model-value="logToggles[stream.id]"
          @change="toggleLog(stream.id, $event)"
        >{{ stream.label }}</OneuiCheckbox>
      </div>
      <div class="admin-log-tabs">
        <button
          v-for="stream in logStreams"
          :key="stream.id"
          :class="['admin-tab', activeLogTab === stream.id ? 'is-active' : '']"
          @click="activeLogTab = stream.id"
        >{{ stream.label }}</button>
      </div>
      <div class="actions-row mt-2">
        <SamsungButton
          variant="secondary"
          :disabled="!logsText[activeLogTab]"
          @click="clearActiveLog"
        >
          <template #icon><Trash2 class="button-icon" aria-hidden="true" /></template>
          Очистить лог
        </SamsungButton>
      </div>
      <pre v-if="logToggles[activeLogTab]" class="admin-log-pane">{{ logsText[activeLogTab] || "(пусто)" }}</pre>
      <p v-else class="admin-muted">Поток отключён администратором — переключите чекбокс выше, чтобы возобновить.</p>
    </div>

    <div v-for="backend in backendTabIds" :key="backend" v-show="activeTab === backend" class="surface-card admin-tab-pane">
      <div class="admin-sticky-bar">
        <p class="admin-muted">Настройки только для {{ backendTabLabel(backend) }}.</p>
        <SamsungButton :busy="busyPush" @click="pushConfig">
          <template #icon><UploadCloud class="button-icon" aria-hidden="true" /></template>
          {{ busyPush ? "Отправляем…" : "Применить (Push)" }}
        </SamsungButton>
      </div>
      <ConfigFormEditor
        :model-value="formValue"
        :sections="backendTabSections[backend]"
        :has-root-access="!!detail.client?.has_root_access"
        @update:model-value="onFormChanged"
      />
    </div>

    <div v-if="activeTab === 'xray_profiles'" class="surface-card admin-tab-pane">
      <div class="admin-sticky-bar">
        <p class="admin-muted">VLESS-профили клиента.</p>
        <SamsungButton :busy="busyPush" @click="pushConfig">
          <template #icon><UploadCloud class="button-icon" aria-hidden="true" /></template>
          {{ busyPush ? "Отправляем…" : "Применить (Push)" }}
        </SamsungButton>
      </div>
      <details class="admin-config-import">
        <summary>Добавить из vless:// или wingsv://</summary>
        <textarea
          v-model.trim="profileImportDraft"
          class="text-input admin-import-area"
          rows="3"
          spellcheck="false"
          placeholder="vless://... или wingsv://..."
        />
        <p v-if="profileImportError" class="admin-error">{{ profileImportError }}</p>
        <div class="actions-row mt-2">
          <SamsungButton
            variant="secondary"
            :busy="busyProfileImport"
            :disabled="!profileImportDraft"
            @click="importProfile"
          >
            <template #icon><Link2 class="button-icon" aria-hidden="true" /></template>
            {{ busyProfileImport ? "Распаковываем…" : "Добавить" }}
          </SamsungButton>
        </div>
      </details>
      <p class="admin-muted mt-3" v-if="xrayProfiles.length">
        Активный профиль применится на устройстве после нажатия «Применить (Push)».
      </p>
      <div v-if="xrayProfileFilterOptions.length > 1" class="mt-3">
        <OneuiRadioGroup
          v-model="xrayActiveFilter"
          :options="xrayProfileFilterOptions"
          variant="pill"
        />
      </div>
      <div v-if="xraySubscriptions.length" class="actions-row mt-3 mb-4">
        <SamsungButton
          v-if="xrayActiveFilter !== 'all' && xrayActiveFilter !== '__standalone'"
          variant="secondary"
          :busy="busyRefresh === xrayActiveFilter"
          @click="refreshSubscription(xrayActiveFilter)"
        >
          <template #icon><RotateCw class="button-icon" aria-hidden="true" /></template>
          Обновить эту подписку
        </SamsungButton>
        <SamsungButton variant="secondary" :busy="busyRefresh === 'all'" @click="refreshAllSubscriptions">
          <template #icon><RotateCw class="button-icon" aria-hidden="true" /></template>
          Обновить все
        </SamsungButton>
      </div>
      <ul v-if="paginatedXrayProfiles.length" class="admin-list">
        <li
          v-for="profile in paginatedXrayProfiles"
          :key="profile.id"
          :class="['admin-list-item admin-profile-row', xrayActiveProfileId === profile.id ? 'is-selected' : '']"
          @click="setActiveProfile(profile.id)"
        >
          <div class="admin-profile-text">
            <strong>{{ profile.title || profile.address || profile.id }}</strong>
            <span class="admin-muted">
              {{ profile.address || "—" }}<template v-if="profile.port">:{{ profile.port }}</template>
            </span>
          </div>
          <span class="admin-radio" :class="{ 'is-selected': xrayActiveProfileId === profile.id }" aria-hidden="true">
            <span class="admin-radio-dot"></span>
          </span>
          <div class="admin-list-actions" @click.stop>
            <SamsungIconButton size="small" :title="'Скопировать ссылку'" @click="copyProfileLink(profile)">
              <Check v-if="copiedProfileId === profile.id" class="button-icon" aria-hidden="true" />
              <Copy v-else class="button-icon" aria-hidden="true" />
            </SamsungIconButton>
            <SamsungIconButton size="small" :title="'Удалить'" @click="removeProfile(profile.id)">
              <Trash2 class="button-icon" aria-hidden="true" />
            </SamsungIconButton>
          </div>
        </li>
      </ul>
      <div v-if="filteredXrayProfiles.length > xrayProfilesPageSize" class="admin-pagination">
        <SamsungButton
          variant="secondary"
          :disabled="xrayProfilesPage === 0"
          @click="xrayProfilesPage = Math.max(0, xrayProfilesPage - 1)"
        >
          <template #icon><ChevronLeft class="button-icon" aria-hidden="true" /></template>
          Назад
        </SamsungButton>
        <span class="admin-muted">
          {{ xrayProfilesPage + 1 }} / {{ xrayProfilesTotalPages }}
          ·
          {{ filteredXrayProfiles.length }} {{ pluralProfiles(filteredXrayProfiles.length) }}
        </span>
        <SamsungButton
          variant="secondary"
          :disabled="xrayProfilesPage + 1 >= xrayProfilesTotalPages"
          @click="xrayProfilesPage = Math.min(xrayProfilesTotalPages - 1, xrayProfilesPage + 1)"
        >
          Вперёд
          <ChevronRight class="button-icon" aria-hidden="true" />
        </SamsungButton>
      </div>
      <p v-if="!filteredXrayProfiles.length" class="admin-muted mt-3">Профилей нет.</p>
    </div>

    <div v-if="activeTab === 'xray_subscriptions'" class="surface-card admin-tab-pane">
      <p class="admin-muted">Подписки на Xray-профили (refresh-интервал в минутах, авто-обновление).</p>
      <div class="actions-row mt-3">
        <SamsungButton variant="secondary" @click="addSubscription">
          <template #icon><Plus class="button-icon" aria-hidden="true" /></template>
          Добавить подписку
        </SamsungButton>
      </div>
      <ul v-if="xraySubscriptions.length" class="admin-list mt-3">
        <li v-for="(sub, idx) in xraySubscriptions" :key="sub.id || idx" class="admin-list-item">
          <div class="form-section subscription-card">
            <div class="form-row form-row-stack">
              <label class="form-label">Имя</label>
              <input class="text-input" :value="sub.title || ''" @input="patchSubscription(idx, 'title', $event.target.value)" />
            </div>
            <div class="form-row form-row-stack">
              <label class="form-label">URL</label>
              <input class="text-input" :value="sub.url || ''" @input="patchSubscription(idx, 'url', $event.target.value)" />
            </div>
            <div class="form-row">
              <label class="form-label">Авто-обновление</label>
              <OneuiSwitch :model-value="!!sub.autoUpdate" @change="patchSubscription(idx, 'autoUpdate', $event)" />
            </div>
            <div class="form-row">
              <label class="form-label">Интервал (минут)</label>
              <input
                class="text-input form-input-narrow"
                :value="sub.refreshIntervalMinutes || ''"
                @input="patchSubscription(idx, 'refreshIntervalMinutes', toIntOrUndef($event.target.value))"
                inputmode="numeric"
              />
            </div>
            <div class="form-row" v-if="sub.lastUpdatedAt">
              <label class="form-label">Последнее обновление</label>
              <span class="admin-muted">{{ formatTs(new Date(Number(sub.lastUpdatedAt) * 1000).toISOString()) }}</span>
            </div>
            <div class="actions-row mt-2">
              <SamsungButton variant="secondary" @click="removeSubscription(idx)">
                <template #icon><Trash2 class="button-icon" aria-hidden="true" /></template>
                Удалить
              </SamsungButton>
            </div>
          </div>
        </li>
      </ul>
      <p v-else class="admin-muted mt-3">Подписок нет.</p>
    </div>

    <div v-if="activeTab === 'commands'" class="surface-card admin-tab-pane">
      <p class="admin-muted">
        Тунель сейчас:
        <strong>{{ runtimeStateLabel }}</strong>{{ detail?.client?.online ? "" : " · клиент оффлайн, команды не дойдут" }}
      </p>
      <div class="admin-cmd-grid">
        <SamsungButton :busy="busyCmd" :disabled="!canStart" @click="sendCommand('start_tunnel')">
          <template #icon><Play class="button-icon" aria-hidden="true" /></template>
          Старт тунеля
        </SamsungButton>
        <SamsungButton variant="secondary" :busy="busyCmd" :disabled="!canStop" @click="sendCommand('stop_tunnel')">
          <template #icon><Square class="button-icon" aria-hidden="true" /></template>
          Стоп тунеля
        </SamsungButton>
        <SamsungButton variant="secondary" :busy="busyCmd" :disabled="!canStop" @click="sendCommand('reconnect')">
          <template #icon><RotateCw class="button-icon" aria-hidden="true" /></template>
          Переподключение
        </SamsungButton>
        <SamsungButton
          variant="secondary"
          :busy="busyCmd"
          :disabled="!detail?.client?.online"
          @click="sendCommand('report_now')"
        >
          <template #icon><Activity class="button-icon" aria-hidden="true" /></template>
          Запросить state
        </SamsungButton>
      </div>
      <p v-if="lastCmdAck" class="admin-muted mt-4">Последний ответ: {{ JSON.stringify(lastCmdAck) }}</p>
    </div>

    <section class="danger-card">
      <h2 class="font-sharp text-[18px] font-bold text-wings-text">Опасная зона</h2>
      <p class="body-copy mt-2">
        Удаление клиента отзывает токен и стирает его конфигурацию. Действие необратимое.
      </p>
      <div class="actions-row mt-4">
        <SamsungButton variant="danger" :busy="busyDelete" @click="onDelete">
          <template #icon><Trash2 class="button-icon" aria-hidden="true" /></template>
          {{ busyDelete ? "Удаляем…" : "Удалить клиента" }}
        </SamsungButton>
      </div>
    </section>
  </div>
</template>

<script setup>
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { Activity, Check, ChevronLeft, ChevronRight, Clock, Copy, Download, Eye, GripVertical, Infinity as InfinityIcon, Link2, Play, Plus, RefreshCw, RotateCcw, RotateCw, Square, Trash2, UploadCloud, X } from "lucide-vue-next";
import OneuiCheckbox from "@/components/controls/OneuiCheckbox.vue";
import OneuiRadioGroup from "@/components/controls/OneuiRadioGroup.vue";
import OneuiSelect from "@/components/controls/OneuiSelect.vue";
import OneuiSwitch from "@/components/controls/OneuiSwitch.vue";
import SamsungButton from "@/components/layout/SamsungButton.vue";
import SamsungIconButton from "@/components/layout/SamsungIconButton.vue";
import SamsungLoader from "@/components/layout/SamsungLoader.vue";
import SamsungModal from "@/components/layout/SamsungModal.vue";
import SamsungPill from "@/components/layout/SamsungPill.vue";
import ConfigFormEditor from "@/components/domain/ConfigFormEditor.vue";
import CopyableLink from "@/components/domain/CopyableLink.vue";
// CodeMirror — отдельный chunk; грузим только когда юзер реально открыл JSON-таб.
const JsonEditor = defineAsyncComponent(() => import("@/components/domain/JsonEditor.vue"));
import { connectAdminSocket } from "@/stores/admin-socket.js";

const props = defineProps({ id: { type: String, required: true }, tab: { type: String, default: "" } });
const router = useRouter();

const tabs = [
  { id: "config", label: "Конфигурация" },
  { id: "vk_turn", label: "VK TURN" },
  { id: "xray", label: "Xray" },
  { id: "xray_profiles", label: "Xray профили" },
  { id: "xray_subscriptions", label: "Xray подписки" },
  { id: "xray_rules", label: "Xray правила" },
  { id: "wireguard", label: "WireGuard" },
  { id: "amneziawg", label: "AmneziaWG" },
  { id: "wb_stream", label: "WB Stream" },
  { id: "app_routing", label: "Per-app routing" },
  { id: "state", label: "Состояние" },
  { id: "logs", label: "Логи" },
  { id: "commands", label: "Команды" },
];

const backendTabSections = {
  vk_turn: ["vk_turn"],
  xray: ["xray"],
  wireguard: ["wireguard"],
  amneziawg: ["amneziawg"],
  wb_stream: ["wb_stream"],
};
const logStreams = [
  { id: "runtime", label: "Runtime" },
  { id: "proxy", label: "Proxy" },
  { id: "xray", label: "Xray" },
];

const validTabIds = tabs.map((t) => t.id);
const activeTab = computed(() => (validTabIds.includes(props.tab) ? props.tab : "config"));
function setActiveTab(tabId) {
  if (!validTabIds.includes(tabId)) tabId = "config";
  router.replace({ name: "admin-client-detail", params: { id: props.id, tab: tabId } });
}
const configMode = ref("form");
const followClient = ref(true);
const activeLogTab = ref("runtime");
const profileImportDraft = ref("");
const profileImportError = ref("");
const busyProfileImport = ref(false);
const copiedProfileId = ref("");
const busyRefresh = ref("");
const detail = ref(null);
const loadError = ref("");
const configDraft = ref("");
const configError = ref("");
const busyPush = ref(false);
const busyCmd = ref(false);
const busyDelete = ref(false);
const busyLink = ref(false);
const wingsvLink = ref("");
const wingsvLinkQR = ref("");
const showLinkModal = ref(false);
const copiedLink = ref(false);
const busyRotate = ref(false);

async function onRotateToken() {
  if (busyRotate.value) return;
  if (!confirm("Ротировать токен клиента? Текущая wingsv:// ссылка станет недействительной — устройство потеряет связь с панелью, пока новая ссылка не будет применена.")) {
    return;
  }
  busyRotate.value = true;
  try {
    const res = await fetch(`/api/admin/clients/${id.value}/rotate-token`, {
      method: "POST",
      credentials: "include",
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || "Не удалось ротировать токен");
    }
    const body = await res.json();
    wingsvLink.value = body.wingsv_link || "";
    showLinkModal.value = true;
  } catch (err) {
    loadError.value = err.message;
  } finally {
    busyRotate.value = false;
  }
}

const wingsvLinkPreview = computed(() => {
  const link = wingsvLink.value || "";
  if (!link) return "wingsv://...";
  if (link.length <= 56) return link;
  return link.slice(0, 28) + "…" + link.slice(-16);
});

async function copyLinkInline() {
  if (!wingsvLink.value) {
    try {
      await ensureLink();
    } catch {}
  }
  try {
    await navigator.clipboard.writeText(wingsvLink.value);
    copiedLink.value = true;
    setTimeout(() => (copiedLink.value = false), 1500);
  } catch {}
}

const headerInitials = computed(() => {
  const value = (detail.value?.client?.name || id.value || "").trim();
  if (!value) return "·";
  const parts = value.split(/[\s._-]+/).filter(Boolean);
  if (parts.length === 0) return value.slice(0, 2).toUpperCase();
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
});

function formatDate(iso) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleDateString("ru-RU");
  } catch {
    return iso;
  }
}

const connectionTitle = computed(() => {
  const backend = detail.value?.client?.backend_type;
  return backend ? backend : "Конфигурация клиента";
});

const connectionFacts = computed(() => {
  const cfg = detail.value?.reported_config || detail.value?.desired_config || {};
  const client = detail.value?.client || {};
  const rows = [];
  rows.push({ label: "Бэкенд", value: client.backend_type || "—" });
  const backend = String(client.backend_type || "").toLowerCase();
  if (backend.includes("xray")) {
    const xray = cfg.xray || {};
    const profiles = xray.profiles || [];
    const active = profiles.find((p) => p.id === xray.activeProfileId) || profiles[0];
    if (active) {
      rows.push({ label: "Профиль", value: active.title || active.id });
      if (active.address) rows.push({ label: "Address", value: `${active.address}${active.port ? ":" + active.port : ""}`, mono: true });
    }
    const subs = xray.subscriptions || [];
    if (subs.length) rows.push({ label: "Подписок", value: String(subs.length) });
  } else if (backend.includes("vk turn") || backend.includes("vk_turn")) {
    const turn = cfg.turn || {};
    if (turn.endpoint?.host) rows.push({ label: "Endpoint", value: `${turn.endpoint.host}${turn.endpoint.port ? ":" + turn.endpoint.port : ""}`, mono: true });
    const linkCount = (turn.links || []).length + (turn.link ? 1 : 0);
    rows.push({ label: "VK ссылок", value: String(linkCount) });
    if (turn.threads) rows.push({ label: "Threads", value: String(turn.threads) });
  } else if (backend.includes("wireguard") || backend.includes("amneziawg")) {
    const wg = backend.includes("amneziawg") ? cfg.amneziawg : cfg.wireguard;
    const peer = wg?.peer || {};
    const iface = wg?.iface || {};
    if (peer.endpoint?.host) rows.push({ label: "Peer", value: `${peer.endpoint.host}${peer.endpoint.port ? ":" + peer.endpoint.port : ""}`, mono: true });
    if (iface.privateKey) rows.push({ label: "Iface key", value: shortKey(iface.privateKey), mono: true });
    if (peer.publicKey) rows.push({ label: "Peer key", value: shortKey(peer.publicKey), mono: true });
  } else if (backend.includes("wb stream")) {
    const wb = cfg.wbStream || {};
    if (wb.roomId) rows.push({ label: "Room", value: wb.roomId, mono: true });
    if (wb.displayName) rows.push({ label: "Имя", value: wb.displayName });
  }
  rows.push({ label: "Sync", value: syncModeLabel(client.sync_mode) });
  if (client.sync_mode === "periodic") rows.push({ label: "Интервал", value: `${client.periodic_interval_minutes} мин` });
  rows.push({ label: "Создан", value: formatDate(client.created_at) });
  rows.push({ label: "ID", value: client.id, mono: true });
  return rows;
});

function shortKey(value) {
  const v = String(value || "").trim();
  if (v.length <= 14) return v;
  return v.slice(0, 6) + "…" + v.slice(-6);
}

function syncModeLabel(mode) {
  switch (mode) {
    case "periodic": return "Периодически";
    case "foreground": return "Только пока открыто";
    default: return "Всегда в фоне";
  }
}
const importLinkDraft = ref("");
const importError = ref("");
const busyImport = ref(false);
const lastCmdAck = ref(null);
const logToggles = ref({ runtime: true, proxy: true, xray: true });
const logsText = ref({ runtime: "", proxy: "", xray: "" });

let socketHandle = null;

const id = computed(() => props.id);

async function loadDetail() {
  loadError.value = "";
  try {
    const res = await fetch(`/api/admin/clients/${id.value}`, { credentials: "include" });
    if (!res.ok) throw new Error(await res.text());
    detail.value = await res.json();
    if (detail.value.client) {
      logToggles.value = {
        runtime: !!detail.value.client.log_runtime_enabled,
        proxy: !!detail.value.client.log_proxy_enabled,
        xray: !!detail.value.client.log_xray_enabled,
      };
      syncModeDraft.value = detail.value.client.sync_mode || "always";
      syncIntervalDraft.value = detail.value.client.periodic_interval_minutes || 30;
    }
    // Default to the actual current state when the detail page opens — admins
    // overwhelmingly want to see "what does the device have right now" rather
    // than "what did I last push", and the client never streams its desired
    // state back. While followClient is on we keep mirroring it.
    if (followClient.value && detail.value.reported_config) {
      configDraft.value = formatJson(detail.value.reported_config);
    } else {
      configDraft.value = formatJson(detail.value.desired_config) || "{}";
    }
    // Lazy-load the wingsv:// link so the QR card on Конфигурация has data
    // without requiring the admin to click "Показать ссылку" first.
    if (!wingsvLink.value) {
      ensureLink().catch(() => {});
    }
  } catch (err) {
    loadError.value = err.message || "Не удалось загрузить клиента";
  }
}

function setFollowClient(value) {
  followClient.value = value;
  if (value && detail.value?.reported_config) {
    configDraft.value = formatJson(detail.value.reported_config);
  }
}

function formatJson(value) {
  if (value == null) return "";
  try {
    return JSON.stringify(value, null, 2);
  } catch (err) {
    return String(value);
  }
}

function formatTs(iso) {
  if (!iso || iso.startsWith("1970")) return "—";
  try {
    return new Date(iso).toLocaleString("ru-RU");
  } catch {
    return iso;
  }
}

function resetConfigDraft() {
  configDraft.value = formatJson(detail.value?.desired_config) || "{}";
  configError.value = "";
}

const formValue = computed(() => {
  try {
    return JSON.parse(configDraft.value || "{}");
  } catch {
    return {};
  }
});

function onFormChanged(next) {
  followClient.value = false;
  configDraft.value = JSON.stringify(next, null, 2);
  configError.value = "";
}

// Stop following the moment the admin types into the JSON area — we don't
// want the next live update to wipe their in-progress edit.
watch(configDraft, () => {
  if (configMode.value === "json" && followClient.value && detail.value?.reported_config) {
    const reported = formatJson(detail.value.reported_config);
    if (configDraft.value !== reported) {
      followClient.value = false;
    }
  }
});

const runtimeStateLabel = computed(() => {
  const r = detail.value?.runtime;
  if (!r) return "не получено";
  if (r.tunnelActive) {
    if (r.phase && r.phase !== "TUNNEL_PHASE_UNSPECIFIED") {
      return r.phase.replace("TUNNEL_PHASE_", "").toLowerCase();
    }
    return "running";
  }
  return "off";
});

const canStart = computed(() => {
  if (!detail.value?.client?.online) return false;
  return !detail.value?.runtime?.tunnelActive;
});

const canStop = computed(() => {
  if (!detail.value?.client?.online) return false;
  return !!detail.value?.runtime?.tunnelActive;
});

const backendTabIds = Object.keys(backendTabSections);

function backendTabLabel(id) {
  const tab = tabs.find((t) => t.id === id);
  return tab ? tab.label : id;
}

const xrayProfiles = computed(() => formValue.value?.xray?.profiles || []);
const xrayActiveProfileId = computed(() => formValue.value?.xray?.activeProfileId || "");
const xraySubscriptions = computed(() => formValue.value?.xray?.subscriptions || []);

const xrayActiveFilter = ref("all");
const xrayProfilesPage = ref(0);
const xrayProfilesPageSize = 50;
const installedApps = ref([]);
const installedAppsUpdated = ref("");
const syncModeDraft = ref("always");
const syncIntervalDraft = ref(30);
let syncSaveTimer = null;

const syncModeOptions = [
  { id: "always", label: "Всегда в фоне", icon: InfinityIcon },
  { id: "periodic", label: "Периодически", icon: Clock },
  { id: "foreground", label: "Только пока открыто", icon: Eye },
];

function setSyncMode(mode) {
  if (syncModeDraft.value === mode) return;
  syncModeDraft.value = mode;
  saveSyncSettings();
}
const appsFilter = ref("");
const busyAppsRefresh = ref(false);

const appRouting = computed(() => formValue.value?.appRouting || {});

const xrayRouting = computed(() => formValue.value?.xray?.routing || {});
const xrayRules = computed(() => xrayRouting.value.rules || []);
const draggingIdx = ref(-1);
const dragOverIdx = ref(-1);

const matchTypeOptions = [
  { value: "XRAY_ROUTING_MATCH_UNSPECIFIED", label: "—" },
  { value: "XRAY_ROUTING_MATCH_GEOIP", label: "GeoIP" },
  { value: "XRAY_ROUTING_MATCH_GEOSITE", label: "GeoSite" },
  { value: "XRAY_ROUTING_MATCH_DOMAIN", label: "Домен" },
  { value: "XRAY_ROUTING_MATCH_IP", label: "IP / CIDR" },
  { value: "XRAY_ROUTING_MATCH_PORT", label: "Порт" },
];

const actionOptions = [
  { value: "XRAY_ROUTING_ACTION_UNSPECIFIED", label: "—" },
  { value: "XRAY_ROUTING_ACTION_PROXY", label: "Через тунель" },
  { value: "XRAY_ROUTING_ACTION_DIRECT", label: "Напрямую" },
  { value: "XRAY_ROUTING_ACTION_BLOCK", label: "Блокировать" },
];

function matchCodeLabel(matchType) {
  switch (matchType) {
    case "XRAY_ROUTING_MATCH_GEOIP": return "GeoIP код (например, geoip:cn)";
    case "XRAY_ROUTING_MATCH_GEOSITE": return "GeoSite код (например, geosite:google)";
    case "XRAY_ROUTING_MATCH_DOMAIN": return "Домен";
    case "XRAY_ROUTING_MATCH_IP": return "IP или CIDR";
    case "XRAY_ROUTING_MATCH_PORT": return "Порт или диапазон (например, 80,443,8000-9000)";
    default: return "Значение";
  }
}

function matchCodePlaceholder(matchType) {
  switch (matchType) {
    case "XRAY_ROUTING_MATCH_GEOIP": return "cn";
    case "XRAY_ROUTING_MATCH_GEOSITE": return "google";
    case "XRAY_ROUTING_MATCH_DOMAIN": return "example.com";
    case "XRAY_ROUTING_MATCH_IP": return "10.0.0.0/8";
    case "XRAY_ROUTING_MATCH_PORT": return "443";
    default: return "";
  }
}

function setXrayRouting(key, value) {
  const next = { ...xrayRouting.value };
  if (value === undefined || value === null || value === "") delete next[key];
  else next[key] = value;
  patchXray({ routing: next });
}

function setXrayRules(rules) {
  patchXray({ routing: { ...xrayRouting.value, rules } });
}

function generateRuleId() {
  if (typeof crypto !== "undefined" && crypto.randomUUID) return crypto.randomUUID();
  return "r-" + Math.random().toString(36).slice(2, 10);
}

function addRule() {
  const next = [...xrayRules.value, {
    id: generateRuleId(),
    matchType: "XRAY_ROUTING_MATCH_DOMAIN",
    code: "",
    action: "XRAY_ROUTING_ACTION_PROXY",
    enabled: true,
  }];
  setXrayRules(next);
}

function removeRule(idx) {
  const next = xrayRules.value.filter((_, i) => i !== idx);
  setXrayRules(next);
}

function patchRule(idx, key, value) {
  const next = xrayRules.value.map((r, i) => {
    if (i !== idx) return r;
    const updated = { ...r };
    if (value === undefined || value === null || value === "") delete updated[key];
    else updated[key] = value;
    return updated;
  });
  setXrayRules(next);
}

function onRuleDragStart(idx, ev) {
  draggingIdx.value = idx;
  if (ev.dataTransfer) {
    ev.dataTransfer.effectAllowed = "move";
    ev.dataTransfer.setData("text/plain", String(idx));
  }
}

function onRuleDragOver(idx, ev) {
  if (draggingIdx.value < 0 || draggingIdx.value === idx) return;
  ev.dataTransfer.dropEffect = "move";
  dragOverIdx.value = idx;
}

function onRuleDrop(targetIdx) {
  const from = draggingIdx.value;
  if (from < 0 || from === targetIdx) {
    onRuleDragEnd();
    return;
  }
  const arr = [...xrayRules.value];
  const [moved] = arr.splice(from, 1);
  arr.splice(targetIdx, 0, moved);
  setXrayRules(arr);
  onRuleDragEnd();
}

function onRuleDragEnd() {
  draggingIdx.value = -1;
  dragOverIdx.value = -1;
}

function setAppRoutingField(key, value) {
  const next = { ...appRouting.value };
  if (value === undefined || value === null || value === "") delete next[key];
  else next[key] = value;
  onFormChanged({ ...(formValue.value || {}), appRouting: next });
}

function setAppRoutingPackages(text) {
  const arr = String(text || "")
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
  setAppRoutingField("packages", arr.length ? arr : undefined);
}

function isPackageRouted(pkg) {
  return Array.isArray(appRouting.value.packages) && appRouting.value.packages.includes(pkg);
}

function togglePackageRouted(pkg, on) {
  const cur = new Set(appRouting.value.packages || []);
  if (on) cur.add(pkg);
  else cur.delete(pkg);
  setAppRoutingField("packages", cur.size ? [...cur] : undefined);
}

const appsKindFilter = ref("all");

const installedAppsCounts = computed(() => {
  const all = installedApps.value;
  return {
    all: all.length,
    recommended: all.filter((a) => a.recommended).length,
    user: all.filter((a) => !a.system).length,
    system: all.filter((a) => a.system).length,
  };
});

const appsKindFilterOptions = computed(() => [
  { value: "all", label: "Все", count: installedAppsCounts.value.all },
  { value: "recommended", label: "Рекомендуемые", count: installedAppsCounts.value.recommended },
  { value: "user", label: "Пользовательские", count: installedAppsCounts.value.user },
  { value: "system", label: "Системные", count: installedAppsCounts.value.system },
]);

const filteredInstalledApps = computed(() => {
  const filter = appsFilter.value.toLowerCase();
  const kind = appsKindFilter.value;
  return installedApps.value.filter((app) => {
    if (kind === "recommended" && !app.recommended) return false;
    if (kind === "user" && app.system) return false;
    if (kind === "system" && !app.system) return false;
    if (
      filter &&
      !(app.label || "").toLowerCase().includes(filter) &&
      !(app.package || "").toLowerCase().includes(filter)
    ) {
      return false;
    }
    return true;
  });
});

function saveSyncSettings() {
  if (syncSaveTimer) clearTimeout(syncSaveTimer);
  syncSaveTimer = setTimeout(async () => {
    try {
      await fetch(`/api/admin/clients/${id.value}/sync`, {
        method: "PUT",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          sync_mode: syncModeDraft.value,
          periodic_interval_minutes: Math.max(15, Number(syncIntervalDraft.value) || 30),
        }),
      });
    } catch (err) {
      console.warn("saveSyncSettings", err);
    }
  }, 300);
}

async function loadInstalledApps() {
  try {
    const res = await fetch(`/api/admin/clients/${id.value}/installed-apps`, { credentials: "include" });
    if (!res.ok) return;
    const body = await res.json();
    installedApps.value = body.apps || [];
    installedAppsUpdated.value = body.updated_at || "";
  } catch {}
}

async function refreshInstalledApps() {
  if (busyAppsRefresh.value) return;
  busyAppsRefresh.value = true;
  try {
    await fetch(`/api/admin/clients/${id.value}/installed-apps/refresh`, {
      method: "POST",
      credentials: "include",
    });
    setTimeout(loadInstalledApps, 1500);
  } catch {} finally {
    busyAppsRefresh.value = false;
  }
}

const xrayProfileFilterOptions = computed(() => {
  const standaloneCount = xrayProfiles.value.filter((p) => !p.subscriptionId).length;
  const subCounts = new Map();
  for (const p of xrayProfiles.value) {
    if (!p.subscriptionId) continue;
    subCounts.set(p.subscriptionId, (subCounts.get(p.subscriptionId) || 0) + 1);
  }
  const opts = [{ value: "all", label: "Все", count: xrayProfiles.value.length }];
  if (standaloneCount > 0) {
    opts.push({ value: "__standalone", label: "Без подписки", count: standaloneCount });
  }
  for (const sub of xraySubscriptions.value) {
    opts.push({ value: sub.id, label: sub.title || sub.id, count: subCounts.get(sub.id) || 0 });
  }
  return opts;
});

const filteredXrayProfiles = computed(() => {
  if (xrayActiveFilter.value === "all") return xrayProfiles.value;
  if (xrayActiveFilter.value === "__standalone") {
    return xrayProfiles.value.filter((p) => !p.subscriptionId);
  }
  return xrayProfiles.value.filter((p) => p.subscriptionId === xrayActiveFilter.value);
});

const xrayProfilesTotalPages = computed(() =>
  Math.max(1, Math.ceil(filteredXrayProfiles.value.length / xrayProfilesPageSize))
);

const paginatedXrayProfiles = computed(() => {
  const start = xrayProfilesPage.value * xrayProfilesPageSize;
  return filteredXrayProfiles.value.slice(start, start + xrayProfilesPageSize);
});

watch(xrayActiveFilter, () => {
  xrayProfilesPage.value = 0;
});

watch(xrayProfilesTotalPages, (total) => {
  if (xrayProfilesPage.value >= total) xrayProfilesPage.value = Math.max(0, total - 1);
});

function pluralProfiles(n) {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return "профиль";
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) return "профиля";
  return "профилей";
}

async function copyProfileLink(profile) {
  if (!profile?.rawLink) return;
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(profile.rawLink);
    } else {
      const ta = document.createElement("textarea");
      ta.value = profile.rawLink;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
    }
    copiedProfileId.value = profile.id;
    setTimeout(() => {
      if (copiedProfileId.value === profile.id) copiedProfileId.value = "";
    }, 1200);
  } catch (err) {
    console.warn("copyProfileLink", err);
  }
}

async function refreshSubscription(subscriptionId) {
  busyRefresh.value = subscriptionId;
  try {
    const res = await fetch(`/api/admin/clients/${id.value}/refresh-subscription`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ subscription_id: subscriptionId }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || "Не удалось отправить команду");
    }
  } catch (err) {
    loadError.value = err.message;
  } finally {
    busyRefresh.value = "";
  }
}

async function refreshAllSubscriptions() {
  busyRefresh.value = "all";
  try {
    const res = await fetch(`/api/admin/clients/${id.value}/refresh-subscription`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || "Не удалось отправить команду");
    }
  } catch (err) {
    loadError.value = err.message;
  } finally {
    busyRefresh.value = "";
  }
}

function patchXray(patch) {
  const next = { ...(formValue.value || {}) };
  next.xray = { ...(next.xray || {}), ...patch };
  onFormChanged(next);
}

function setActiveProfile(id) {
  patchXray({ activeProfileId: id });
}

function removeProfile(id) {
  const profiles = xrayProfiles.value.filter((p) => p.id !== id);
  const next = { ...(formValue.value?.xray || {}), profiles };
  if (next.activeProfileId === id) next.activeProfileId = profiles[0]?.id || "";
  patchXray(next);
}

async function importProfile() {
  if (!profileImportDraft.value || busyProfileImport.value) return;
  busyProfileImport.value = true;
  profileImportError.value = "";
  try {
    const res = await fetch("/api/admin/decode-link", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ link: profileImportDraft.value }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || "Не удалось распаковать ссылку");
    }
    const body = await res.json();
    const newProfiles = body.config?.xray?.profiles || [];
    if (!newProfiles.length) {
      throw new Error("Ссылка не содержит Xray-профилей");
    }
    const merged = [...xrayProfiles.value];
    for (const p of newProfiles) {
      if (!merged.some((existing) => existing.id === p.id)) merged.push(p);
    }
    const xray = {
      ...(formValue.value?.xray || {}),
      profiles: merged,
    };
    if (!xray.activeProfileId && merged.length) xray.activeProfileId = merged[0].id;
    patchXray(xray);
    profileImportDraft.value = "";
  } catch (err) {
    profileImportError.value = err.message;
  } finally {
    busyProfileImport.value = false;
  }
}

function addSubscription() {
  const id = "sub_" + Math.random().toString(36).slice(2, 10);
  const subs = [...xraySubscriptions.value, { id, title: "Новая подписка", url: "", autoUpdate: false, refreshIntervalMinutes: 60 }];
  patchXray({ subscriptions: subs });
}

function removeSubscription(idx) {
  const subs = xraySubscriptions.value.filter((_, i) => i !== idx);
  patchXray({ subscriptions: subs });
}

function patchSubscription(idx, key, value) {
  const subs = xraySubscriptions.value.map((s, i) => {
    if (i !== idx) return s;
    const next = { ...s };
    if (value === undefined || value === null || value === "") delete next[key];
    else next[key] = value;
    return next;
  });
  patchXray({ subscriptions: subs });
}

function toIntOrUndef(text) {
  if (text === "" || text == null) return undefined;
  const n = Number(text);
  return Number.isFinite(n) ? n : undefined;
}

function setConfigMode(mode) {
  if (mode === "form") {
    // Validate the JSON before switching — fall back to JSON view if invalid.
    try {
      JSON.parse(configDraft.value || "{}");
    } catch (err) {
      configError.value = "JSON невалиден, переключение в форму невозможно: " + err.message;
      return;
    }
  }
  configMode.value = mode;
  configError.value = "";
}

function loadFromReported() {
  if (!detail.value?.reported_config) {
    configError.value = "Клиент ещё не присылал свою конфигурацию";
    return;
  }
  configDraft.value = formatJson(detail.value.reported_config);
  configError.value = "";
}

async function pushConfig() {
  configError.value = "";
  let parsed;
  try {
    parsed = JSON.parse(configDraft.value || "{}");
  } catch (err) {
    configError.value = "Невалидный JSON: " + err.message;
    return;
  }
  busyPush.value = true;
  try {
    const res = await fetch(`/api/admin/clients/${id.value}/config`, {
      method: "PUT",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ config: parsed }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || "Не удалось применить");
    }
    await loadDetail();
  } catch (err) {
    configError.value = err.message;
  } finally {
    busyPush.value = false;
  }
}

async function sendCommand(type) {
  busyCmd.value = true;
  lastCmdAck.value = null;
  try {
    const res = await fetch(`/api/admin/clients/${id.value}/command`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ type }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || "Команда отклонена");
    }
    lastCmdAck.value = await res.json();
  } catch (err) {
    lastCmdAck.value = { ok: false, error: err.message };
  } finally {
    busyCmd.value = false;
  }
}

async function toggleLog(streamId, enabled) {
  logToggles.value = { ...logToggles.value, [streamId]: enabled };
  try {
    await fetch(`/api/admin/clients/${id.value}/log-control`, {
      method: "PUT",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        runtime: logToggles.value.runtime,
        proxy: logToggles.value.proxy,
        xray: logToggles.value.xray,
      }),
    });
  } catch (err) {
    console.warn("toggleLog", err);
  }
}

async function onDelete() {
  if (!confirm("Удалить клиента и отозвать его токен?")) return;
  busyDelete.value = true;
  try {
    const res = await fetch(`/api/admin/clients/${id.value}`, {
      method: "DELETE",
      credentials: "include",
    });
    if (!res.ok) throw new Error(await res.text());
    router.push({ name: "admin-clients" });
  } catch (err) {
    loadError.value = err.message;
  } finally {
    busyDelete.value = false;
  }
}

async function showLink() {
  busyLink.value = true;
  try {
    await ensureLink();
    showLinkModal.value = true;
  } catch (err) {
    loadError.value = err.message;
  } finally {
    busyLink.value = false;
  }
}

async function ensureLink() {
  if (wingsvLink.value) return wingsvLink.value;
  const res = await fetch(`/api/admin/clients/${id.value}/wingsv-link`, { credentials: "include" });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message || "Ссылка недоступна");
  }
  const data = await res.json();
  wingsvLink.value = data.wingsv_link;
  return data.wingsv_link;
}

function dismissLink() {
  showLinkModal.value = false;
}

async function generateQR(link) {
  if (!link) return "";
  const QR = await import("qrcode");
  const canvas = document.createElement("canvas");
  canvas.width = 320;
  canvas.height = 320;
  await QR.toCanvas(canvas, link, {
    errorCorrectionLevel: "H",
    width: 320,
    margin: 1,
    color: { dark: "#000000", light: "#ffffff" },
  });
  // Overlay WINGS V app icon in the center.
  const ctx = canvas.getContext("2d");
  const icon = new Image();
  icon.src = "/img/wingsv-icon.webp";
  await new Promise((resolve) => {
    icon.onload = resolve;
    icon.onerror = resolve;
  });
  const size = 64;
  const x = (canvas.width - size) / 2;
  const y = (canvas.height - size) / 2;
  ctx.fillStyle = "#ffffff";
  ctx.beginPath();
  ctx.roundRect(x - 6, y - 6, size + 12, size + 12, 16);
  ctx.fill();
  ctx.save();
  ctx.beginPath();
  ctx.roundRect(x, y, size, size, 14);
  ctx.clip();
  ctx.drawImage(icon, x, y, size, size);
  ctx.restore();
  return canvas.toDataURL("image/png");
}

watch(wingsvLink, async (link) => {
  wingsvLinkQR.value = link ? await generateQR(link) : "";
});

async function importFromLink() {
  if (!importLinkDraft.value || busyImport.value) return;
  busyImport.value = true;
  importError.value = "";
  try {
    const res = await fetch("/api/admin/decode-link", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ link: importLinkDraft.value }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.message || "Не удалось распаковать ссылку");
    }
    const body = await res.json();
    let current = {};
    try {
      current = JSON.parse(configDraft.value || "{}");
    } catch {
      current = {};
    }
    const merged = mergeConfig(current, body.config || {});
    configDraft.value = JSON.stringify(merged, null, 2);
    importLinkDraft.value = "";
  } catch (err) {
    importError.value = err.message;
  } finally {
    busyImport.value = false;
  }
}

function mergeConfig(base, patch) {
  if (patch == null) return base;
  if (Array.isArray(patch)) return patch;
  if (typeof patch !== "object") return patch;
  if (base == null || typeof base !== "object" || Array.isArray(base)) {
    return { ...patch };
  }
  const out = { ...base };
  for (const key of Object.keys(patch)) {
    const pv = patch[key];
    if (pv === undefined) continue;
    if (pv && typeof pv === "object" && !Array.isArray(pv)) {
      out[key] = mergeConfig(base[key], pv);
    } else {
      out[key] = pv;
    }
  }
  return out;
}

function clearActiveLog() {
  // Чистим только локальный буфер — устройство продолжит присылать новые
  // log_chunk-события и панель снова заполнится с текущей точки.
  logsText.value = { ...logsText.value, [activeLogTab.value]: "" };
}

function appendLogChunk(streamName, chunk) {
  if (!logToggles.value[streamName]) return;
  const lines = (chunk?.lines || []).map((l) => l.text).join("\n");
  if (!lines) return;
  const tail = logsText.value[streamName] || "";
  const next = tail ? tail + "\n" + lines : lines;
  // Keep only the last ~64 KB of text per stream so the textarea stays snappy.
  logsText.value = { ...logsText.value, [streamName]: next.slice(-65536) };
}

function streamFromInt(value) {
  switch (value) {
    case 1:
    case "LOG_STREAM_RUNTIME":
      return "runtime";
    case 2:
    case "LOG_STREAM_PROXY":
      return "proxy";
    case 3:
    case "LOG_STREAM_XRAY":
      return "xray";
    default:
      return null;
  }
}

watch(id, () => {
  loadDetail();
  loadInstalledApps();
});

onMounted(() => {
  loadDetail();
  loadInstalledApps();
  socketHandle = connectAdminSocket((event) => {
    if (event.client_id !== id.value) return;
    if (event.kind === "status_update" || event.kind === "error") {
      loadDetail();
    } else if (event.kind === "state_report") {
      loadDetail();
    } else if (event.kind === "log_chunk") {
      const streamName = streamFromInt(event.payload?.stream);
      if (streamName) appendLogChunk(streamName, event.payload);
    } else if (event.kind === "command_ack") {
      lastCmdAck.value = event.payload;
    } else if (event.kind === "installed_apps") {
      loadInstalledApps();
    }
  });
});

onBeforeUnmount(() => {
  if (socketHandle) socketHandle.close();
});
</script>
