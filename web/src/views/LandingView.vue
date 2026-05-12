<template>
  <!-- ============================================================
       LANDING / PRE-LOGIN — Samsung Account-style hero, then dark
       cards with the same vocabulary as the Account dashboard.
       ============================================================ -->
  <div class="landing-stage">
    <header class="samsung-topbar">
      <router-link class="samsung-topbar-brand" to="/">
        <span class="wordmark-inline">WINGS V</span>
      </router-link>
      <router-link class="samsung-topbar-link" :to="{ name: 'login' }">
        Войти в панель
      </router-link>
    </header>

    <!-- Big-headline hero. Headline + sub-line on the left, primary
         pill CTA below; right column reserved for product art. -->
    <section class="landing-hero">
      <div class="landing-hero-inner">
        <div class="landing-hero-text">
          <h1 class="landing-hero-headline">
            Все возможности<br />
            <span class="wordmark-inline">WINGS V</span> в одной панели
          </h1>
          <p class="landing-hero-sub">
            Единый клиент для Xray, VK TURN, WireGuard и AmneziaWG —
            теперь с удобным удалённым управлением.
          </p>
          <div class="landing-hero-actions">
            <SamsungButton :to="{ name: 'login' }">Вход</SamsungButton>
            <SamsungButton variant="ghost" href="#download">Скачать APK</SamsungButton>
          </div>
        </div>

        <div class="landing-hero-art" aria-hidden="true">
          <img src="/img/hero-security.png" alt="" class="landing-hero-illustration" />
        </div>
      </div>
    </section>

    <!-- Below-the-fold cards keep the existing functionality but
         re-skinned to the new tokens. -->
    <main class="page-shell">
      <section class="surface-card">
        <h2 class="hero-title">Открытие ссылок</h2>
        <p class="body-copy mt-3">
          Откройте ссылку в <span class="wordmark-inline">WINGS V</span>, посмотрите её содержимое перед импортом
          или скачайте приложение, если оно ещё не установлено.
        </p>

        <div class="entry-card">
          <div class="input-field">
            <label class="field-label" for="link-input">Ссылка WINGS V или VLESS</label>
            <textarea
              id="link-input"
              v-model.trim="linkInput"
              class="link-input"
              placeholder="wingsv://… или vless://…"
              rows="4"
            />
          </div>
          <div class="actions-row">
            <SamsungButton :busy="previewLoading" :disabled="!linkInput" @click="loadPreview">
              <template #icon><Eye class="button-icon" aria-hidden="true" /></template>
              {{ previewLoading ? "Проверяем…" : "Предпросмотр" }}
            </SamsungButton>
            <SamsungButton variant="secondary" :disabled="!openLink" @click="openInApp">
              <template #icon><ExternalLink class="button-icon" aria-hidden="true" /></template>
              Открыть в <span class="wordmark-inline">WINGS V</span>
            </SamsungButton>
          </div>
          <p v-if="previewError" class="state-error">{{ previewError }}</p>
        </div>
      </section>

      <section v-if="preview" class="surface-card">
        <div class="section-head">
          <div>
            <div class="section-kicker">Предпросмотр</div>
            <h2 class="section-title">{{ preview.title }}</h2>
            <p class="body-copy mt-3">{{ preview.subtitle }}</p>
          </div>
          <div class="badge-pill">{{ preview.backend }}</div>
        </div>

        <PreviewFacts :facts="preview.quickFacts" />

        <div v-if="preview.sections && preview.sections.length" class="preview-sections">
          <PreviewSection
            v-for="(section, index) in preview.sections"
            :key="`${section.title}-${index}`"
            :section="section"
          />
        </div>
      </section>

      <section v-else-if="previewLoading" class="surface-card">
        <SamsungSectionLoader />
      </section>

      <section id="download" class="surface-card">
        <div class="section-head">
          <div>
            <div class="section-kicker">Скачать WINGS V</div>
            <h2 class="section-title"><span class="wordmark-inline">WINGS V</span> для Android</h2>
            <p class="body-copy mt-3">
              Загрузите актуальную версию приложения для Android и установите её вручную.
            </p>
          </div>
          <SamsungButton variant="ghost" :busy="releaseLoading" @click="loadRelease">
            <template #icon><RefreshCw class="button-icon" aria-hidden="true" /></template>
            {{ releaseLoading ? "Обновляем…" : "Обновить" }}
          </SamsungButton>
        </div>

        <div v-if="releaseError" class="state-error">{{ releaseError }}</div>
        <div v-else-if="release" class="release-card">
          <div class="release-topline">
            <div>
              <div class="release-tag">{{ release.tagName || release.name }}</div>
              <div class="release-meta">
                {{ release.asset?.name }} · {{ formatBytes(release.asset?.size || 0) }}
              </div>
            </div>
            <SamsungButton variant="ghost" :href="release.htmlUrl" target="_blank" rel="noreferrer">GitHub</SamsungButton>
          </div>

          <div class="download-block">
            <div class="progress-row">
              <div>{{ downloadState.label }}</div>
              <div>{{ downloadState.percent }}%</div>
            </div>
            <div class="progress-track">
              <div class="progress-fill" :style="{ width: `${downloadState.percent}%` }"></div>
            </div>
            <div class="actions-row">
              <SamsungButton :busy="downloading" :disabled="!release" @click="downloadLatest">
                <template #icon>
                  <CheckCircle2 v-if="cacheReady" class="button-icon" aria-hidden="true" />
                  <Download v-else class="button-icon" aria-hidden="true" />
                </template>
                {{ downloading ? "Скачиваем…" : cacheReady ? "Установить" : "Скачать APK" }}
              </SamsungButton>
              <SamsungButton variant="secondary" :disabled="!openLink" @click="openInApp">
                <template #icon><ExternalLink class="button-icon" aria-hidden="true" /></template>
                Открыть <span class="wordmark-inline">WINGS V</span>
              </SamsungButton>
            </div>
          </div>
        </div>
        <SamsungSectionLoader v-else-if="releaseLoading" class="mt-6" />
      </section>
    </main>

    <footer class="landing-footer">
      <span class="wordmark-inline">WINGS V</span>
      <span class="landing-footer-meta">WINGS-N · {{ year }} · All rights reserved</span>
    </footer>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from "vue";
import { CheckCircle2, Download, ExternalLink, Eye, RefreshCw } from "lucide-vue-next";
import SamsungButton from "@/components/layout/SamsungButton.vue";
import SamsungSectionLoader from "@/components/layout/SamsungSectionLoader.vue";
import PreviewFacts from "@/components/domain/PreviewFacts.vue";
import PreviewSection from "@/components/domain/PreviewSection.vue";

const CACHE_NAME = "wingsv-download-cache-v1";
const RELEASE_API_URL = "/api/releases/latest";
const RELEASE_DOWNLOAD_URL = "/api/download/latest";
const APK_CONTENT_TYPE = "application/vnd.android.package-archive";

const params = new URLSearchParams(window.location.search);
const initialLink = params.get("link") || "";

const linkInput = ref(initialLink);
const preview = ref(null);
const previewError = ref("");
const previewLoading = ref(false);

const release = ref(null);
const releaseError = ref("");
const releaseLoading = ref(false);
const downloading = ref(false);
const cacheReady = ref(false);
const downloadState = reactive({
  label: "Готово к скачиванию",
  percent: 0,
});

const year = computed(() => new Date().getFullYear());

const openLink = computed(() => {
  const raw = linkInput.value?.trim();
  return raw ? raw : "";
});

onMounted(async () => {
  await loadRelease();
  await checkCachedAsset();
  if (linkInput.value) {
    await loadPreview();
  }
});

async function loadPreview() {
  previewLoading.value = true;
  previewError.value = "";
  preview.value = null;
  try {
    const response = await fetch(`/api/preview?link=${encodeURIComponent(linkInput.value)}`);
    const payload = await response.json();
    if (!response.ok) {
      throw new Error(payload.message || "Не удалось прочитать ссылку");
    }
    preview.value = payload;
  } catch (error) {
    previewError.value = error.message || "Не удалось показать preview";
  } finally {
    previewLoading.value = false;
  }
}

async function loadRelease() {
  releaseLoading.value = true;
  releaseError.value = "";
  try {
    const response = await fetch(RELEASE_API_URL);
    const payload = await response.json();
    if (!response.ok) {
      throw new Error(payload.message || "Не удалось получить релиз");
    }
    const asset = payload.asset || {};
    if (!asset.name) {
      throw new Error("Не удалось найти APK");
    }
    release.value = {
      tagName: payload.tagName,
      name: payload.name,
      publishedAt: payload.publishedAt,
      htmlUrl: payload.htmlUrl,
      body: payload.body,
      repo: payload.repo,
      asset: {
        name: asset.name,
        size: asset.size,
        contentType: asset.content_type || APK_CONTENT_TYPE,
        downloadUrl: RELEASE_DOWNLOAD_URL,
      },
    };
  } catch (error) {
    releaseError.value = error.message || "Не удалось получить релиз";
  } finally {
    releaseLoading.value = false;
  }
}

function openInApp() {
  if (!openLink.value) return;
  window.location.href = openLink.value;
}

async function downloadLatest() {
  if (cacheReady.value) {
    await installFromCache();
    return;
  }
  downloading.value = true;
  downloadState.label = "Скачиваем APK";
  downloadState.percent = 0;
  try {
    const downloadUrl = currentDownloadUrl();
    const cacheKey = currentCacheKey();
    if (!downloadUrl || !cacheKey) throw new Error("Не удалось найти APK");
    const response = await fetch(downloadUrl);
    if (!response.ok || !response.body) throw new Error("Не удалось скачать APK");

    // Prefer the asset size known from the release metadata over Content-Length:
    // proxies (Traefik / HTTP/2) often strip the latter, leaving the bar stuck at 0.
    const headerTotal = Number(response.headers.get("Content-Length") || "0");
    const assetTotal = Number(release.value?.asset?.size || 0);
    const total = headerTotal > 0 ? headerTotal : assetTotal;
    const reader = response.body.getReader();
    const chunks = [];
    let received = 0;
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
      received += value.length;
      if (total > 0) {
        downloadState.percent = Math.min(99, Math.round((received / total) * 100));
      } else {
        // Без known total — показываем хотя бы пройденные байты для feedback.
        downloadState.percent = Math.min(95, Math.floor(received / (1024 * 1024)) * 5);
      }
    }
    const blob = new Blob(chunks, { type: response.headers.get("Content-Type") || APK_CONTENT_TYPE });
    const cachedResponse = new Response(blob, {
      headers: { "Content-Type": blob.type, "Content-Length": String(blob.size) },
    });
    const cache = await caches.open(CACHE_NAME);
    await pruneOtherCachedVersions(cache, cacheKey);
    await cache.put(cacheKey, cachedResponse.clone());
    cacheReady.value = true;
    downloadState.label = "APK готов к установке";
    downloadState.percent = 100;
  } catch (error) {
    downloadState.label = error.message || "Скачивание не удалось";
    downloadState.percent = 0;
  } finally {
    downloading.value = false;
  }
}

async function installFromCache() {
  const cacheKey = currentCacheKey();
  if (!cacheKey) {
    cacheReady.value = false;
    downloadState.label = "Не удалось найти APK";
    downloadState.percent = 0;
    return;
  }
  const cache = await caches.open(CACHE_NAME);
  const response = await cache.match(cacheKey);
  if (!response) {
    cacheReady.value = false;
    downloadState.label = "Кеш не найден, скачайте файл заново";
    downloadState.percent = 0;
    return;
  }
  const blob = await response.blob();
  downloadState.label = "APK открыт из browser cache";
  downloadState.percent = 100;
  await triggerBlobDownload(blob, release.value?.asset?.name || "WINGSV.apk");
}

async function checkCachedAsset() {
  const cacheKey = currentCacheKey();
  if (!cacheKey) {
    cacheReady.value = false;
    return;
  }
  const cache = await caches.open(CACHE_NAME);
  // Drop stale APKs cached under a different version tag.
  await pruneOtherCachedVersions(cache, cacheKey);
  const response = await cache.match(cacheKey);
  cacheReady.value = Boolean(response);
  if (cacheReady.value) {
    downloadState.label = "APK готов к установке";
    downloadState.percent = 100;
  }
}

async function pruneOtherCachedVersions(cache, currentKey) {
  try {
    const keys = await cache.keys();
    for (const req of keys) {
      if (req.url && !req.url.endsWith(currentKey) && currentKey.startsWith("/")) {
        // Compare by full pathname so we don't collide with origin in prefix.
        const url = new URL(req.url);
        const target = new URL(currentKey, url.origin);
        if (url.pathname + url.search !== target.pathname + target.search) {
          await cache.delete(req);
        }
      } else if (req.url && req.url !== currentKey && !req.url.endsWith(currentKey)) {
        await cache.delete(req);
      }
    }
  } catch {}
}

function currentDownloadUrl() {
  return release.value?.asset?.downloadUrl || "";
}

function currentCacheKey() {
  const downloadUrl = currentDownloadUrl();
  if (!downloadUrl) return "";
  const tag = release.value?.tagName || "unknown";
  return `${downloadUrl}?v=${encodeURIComponent(tag)}`;
}

async function triggerBlobDownload(blob, fileName) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = fileName;
  anchor.click();
  setTimeout(() => URL.revokeObjectURL(url), 2000);
}

function formatBytes(size) {
  if (!size) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let index = 0;
  let value = size;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`;
}
</script>

<style scoped>
.landing-stage {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.landing-hero {
  position: relative;
  padding: 32px 24px 80px;
  margin: 0 auto;
  width: 100%;
  max-width: 1240px;
}

@media (min-width: 768px) {
  .landing-hero {
    padding: 48px 40px 120px;
  }
}

.landing-hero-inner {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 40px;
  align-items: center;
}

@media (min-width: 900px) {
  .landing-hero-inner {
    grid-template-columns: minmax(0, 1.05fr) minmax(280px, 0.95fr);
    gap: 64px;
  }
}

.landing-hero-text {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.landing-hero-headline {
  font-family: "SamsungSharpSans", "SamsungOne", sans-serif;
  font-weight: 700;
  font-size: clamp(34px, 5vw, 60px);
  line-height: 1.04;
  letter-spacing: -0.01em;
  color: #fbfbfb;
}

.landing-hero-sub {
  max-width: 56ch;
  font-size: clamp(15px, 1.4vw, 17px);
  line-height: 1.55;
  color: rgba(252, 252, 252, 0.62);
  margin: 0;
}

.landing-hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 8px;
}

.landing-hero-actions .button-primary {
  min-width: 200px;
}

.landing-hero-art {
  display: none;
  position: relative;
  align-items: center;
  justify-content: center;
}

@media (min-width: 900px) {
  .landing-hero-art {
    display: flex;
  }
}

.landing-hero-illustration {
  width: clamp(280px, 32vw, 420px);
  height: auto;
  filter: drop-shadow(0 24px 60px rgba(18, 89, 209, 0.35));
}

.landing-footer {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 12px;
  padding: 32px 40px 40px;
  font-family: "SamsungSharpSans", "SamsungOne", sans-serif;
  font-size: 18px;
  color: rgba(252, 252, 252, 0.4);
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  margin-top: 24px;
}

.landing-footer-meta {
  font-family: "SamsungOne", sans-serif;
  font-size: 12px;
}

@media (max-width: 640px) {
  .landing-footer {
    padding: 22px 20px 28px;
    font-size: 16px;
  }
}
</style>
