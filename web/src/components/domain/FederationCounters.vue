<template>
  <!-- Renders only once real numbers arrive. The feed 404s while the federation
       is off, so an operator who has not opted in sees nothing at all rather
       than a section of zeroes. -->
  <section v-if="live" class="surface-card mt-6">
    <h2 class="hero-title">Федерация</h2>
    <p class="body-copy mt-3">
      Свободный доступ держится на серверах, которые админы отдали в общий пул. Вот что там происходит прямо сейчас.
    </p>

    <div class="admin-stats">
      <div class="stat">
        <span class="stat-kicker">Узлов онлайн</span>
        <span class="stat-value">{{ live.nodes_online }}</span>
        <span class="stat-meta">пожертвовано админами</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Пользователей сейчас</span>
        <span class="stat-value">{{ live.users_online }}</span>
        <span class="stat-meta">активных подключений</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Скорость</span>
        <span class="stat-value">↑ {{ formatRate(live.up_rate_bps) }}</span>
        <span class="stat-meta">↓ {{ formatRate(live.down_rate_bps) }}</span>
      </div>
      <div class="stat">
        <span class="stat-kicker">Передано</span>
        <span class="stat-value">{{ formatBytes(live.up_bytes + live.down_bytes) }}</span>
        <span class="stat-meta">за текущий период</span>
      </div>
    </div>
  </section>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue';

const live = ref(null);
let source = null;

// The browser reconnects an EventSource on its own, so a head restart heals
// without any retry logic here. A 404 (federation off) also ends up in onerror;
// closing there keeps a disabled deployment from reconnecting forever.
onMounted(() => {
  if (typeof EventSource === 'undefined') return;
  source = new EventSource('/api/federation/stats');
  source.onmessage = (event) => {
    try {
      live.value = JSON.parse(event.data);
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
  const units = ['бит/с', 'Кбит/с', 'Мбит/с', 'Гбит/с'];
  let index = 0;
  let value = bits;
  while (value >= 1000 && index < units.length - 1) {
    value /= 1000;
    index += 1;
  }
  return `${value >= 100 || index === 0 ? Math.round(value) : value.toFixed(1)} ${units[index]}`;
}

function formatBytes(size) {
  const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
  let index = 0;
  let value = size || 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value >= 100 || index === 0 ? Math.round(value) : value.toFixed(1)} ${units[index]}`;
}
</script>
