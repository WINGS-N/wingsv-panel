<template>
  <!-- Renders only once real numbers arrive. The feed 404s while the federation
       is off, so an operator who has not opted in sees nothing at all rather
       than a section of zeroes. -->
  <section v-if="live" class="surface-card mt-6">
    <div class="federation-live-head">
      <h2 class="hero-title">Федерация</h2>
      <!-- Просто точка: она говорит "цифры живые" одним своим видом -->
      <span class="live-dot" role="status" :title="`Обновлено ${agoLabel}`" aria-label="Данные обновляются"></span>
    </div>
    <p class="body-copy body-copy-wide mt-3">
      Свободный доступ держится на серверах, которые админы отдали в общий пул. Вот что там происходит прямо сейчас.
    </p>

    <div class="admin-stats">
      <div class="stat">
        <span class="stat-kicker">
          <Server :size="14" class="stat-kicker-icon" aria-hidden="true" />
          Узлов онлайн
        </span>
        <span class="stat-value">{{ live.nodes_online }}</span>
        <span class="stat-meta">пожертвовано админами</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <Users :size="14" class="stat-kicker-icon" aria-hidden="true" />
          Пользователей сейчас
        </span>
        <span class="stat-value">{{ live.users_online }}</span>
        <span class="stat-meta">активных подключений</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <ArrowUp :size="14" class="stat-kicker-icon" :style="{ color: FLOW_UP }" aria-hidden="true" />
          Отдача
        </span>
        <span class="stat-value">{{ formatRate(live.up_rate_bps) }}</span>
        <span class="stat-meta">с узлов федерации</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <ArrowDown :size="14" class="stat-kicker-icon" :style="{ color: FLOW_DOWN }" aria-hidden="true" />
          Приём
        </span>
        <span class="stat-value">{{ formatRate(live.down_rate_bps) }}</span>
        <span class="stat-meta">на устройства</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <CalendarRange :size="14" class="stat-kicker-icon" aria-hidden="true" />
          Передано
        </span>
        <span class="stat-value">{{ formatBytes(live.up_bytes + live.down_bytes) }}</span>
        <span class="stat-meta">за текущий период</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">
          <ArrowUpDown :size="14" class="stat-kicker-icon" aria-hidden="true" />
          Всего за всё время
        </span>
        <span class="stat-value">{{ formatBytes(live.lifetime_bytes) }}</span>
        <span class="stat-meta">с самого запуска федерации</span>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { ArrowDown, ArrowUp, ArrowUpDown, CalendarRange, Server, Users } from 'lucide-vue-next';

// Тот же цветовой код направления, что в приложении: приём зелёный, отдача синяя
const FLOW_DOWN = '#15b76f';
const FLOW_UP = '#0381fe';

const live = ref(null);
const lastAt = ref(0);
let source = null;
let tick = null;

// Сколько прошло с последнего кадра. Точка пульсирует всегда, а это подпись к
// ней: если поток встал, наведение это покажет
const agoLabel = computed(() => {
  if (!lastAt.value) return 'только что';
  const seconds = Math.max(0, Math.round((Date.now() - lastAt.value) / 1000));
  return seconds < 2 ? 'только что' : `${seconds} с назад`;
});

// The browser reconnects an EventSource on its own, so a head restart heals
// without any retry logic here. A 404 (federation off) also ends up in onerror;
// closing there keeps a disabled deployment from reconnecting forever.
onMounted(() => {
  if (typeof EventSource === 'undefined') return;
  source = new EventSource('/api/federation/stats');
  source.onmessage = (event) => {
    try {
      live.value = JSON.parse(event.data);
      lastAt.value = Date.now();
    } catch {
      // A malformed frame is not worth tearing the stream down for.
    }
  };
  source.onerror = () => {
    if (source && source.readyState === EventSource.CLOSED) {
      source = null;
    }
  };
});

onBeforeUnmount(() => {
  if (source) {
    source.close();
    source = null;
  }
});

function formatRate(bytesPerSecond) {
  const bits = (bytesPerSecond || 0) * 8;
  const units = ['bit/s', 'Kbit/s', 'Mbit/s', 'Gbit/s'];
  let index = 0;
  let value = bits;
  while (value >= 1000 && index < units.length - 1) {
    value /= 1000;
    index += 1;
  }
  return `${value >= 100 || index === 0 ? Math.round(value) : value.toFixed(1)} ${units[index]}`;
}

function formatBytes(size) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let index = 0;
  let value = size || 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value >= 100 || index === 0 ? Math.round(value) : value.toFixed(1)} ${units[index]}`;
}
</script>
