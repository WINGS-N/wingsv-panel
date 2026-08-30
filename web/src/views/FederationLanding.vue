<template>
  <!-- ============================================================
       ПРОПАГАНДА ФЕДЕРАЦИИ - отдельная страница, на которую ведут
       из приложения и из панели. Задача одна: объяснить, зачем
       отдавать свой сервер, и показать, что пул живой прямо сейчас.
       ============================================================ -->
  <div class="landing-stage">
    <header class="samsung-topbar">
      <router-link class="samsung-topbar-brand" to="/">
        <span class="wordmark-inline">WINGS V</span>
        <span class="samsung-topbar-divider">|</span>
        <span class="samsung-topbar-tag">Federation</span>
      </router-link>
      <router-link class="samsung-topbar-link" :to="{ name: 'login' }"> Войти в панель </router-link>
    </header>

    <section class="landing-hero">
      <div class="landing-hero-inner">
        <div class="landing-hero-text">
          <span class="fed-hero-kicker">
            <span class="live-dot" aria-hidden="true"></span>
            <span>федерация <span class="wordmark-inline">WINGS V</span></span>
          </span>
          <h1 class="landing-hero-headline">
            <!-- Точка в заголовке - живая: пул работает прямо сейчас, и это
                 видно раньше, чем человек дочитает до цифр -->
            Поделись трафиком<span class="live-dot headline-dot" aria-hidden="true"></span><br />
            во благо народа.
          </h1>
          <p class="landing-hero-sub">
            Свободный интернет не берётся из воздуха - его держат живые серверы живых людей. Отдайте часть трафика
            своего сервера, столько, сколько не жалко, и им будут пользоваться те, кому платный VPN не по карману.
          </p>
          <div class="landing-hero-actions">
            <SamsungButton :to="{ name: 'login' }">Отдать сервер</SamsungButton>
            <SamsungButton variant="ghost" href="#how">Как это работает</SamsungButton>
          </div>
        </div>

        <div class="landing-hero-art" aria-hidden="true">
          <img src="/img/oneui/network.svg" alt="" class="landing-hero-illustration" />
        </div>
      </div>
    </section>

    <main class="page-shell">
      <FederationCounters />

      <section id="how" class="surface-card mt-6">
        <h2 class="hero-title">Как это работает</h2>
        <div class="fed-steps">
          <article class="fed-step">
            <img src="/img/icon-cloud-up.png" alt="" class="fed-step-icon" aria-hidden="true" />
            <h3 class="fed-step-title">Вы называете лимит</h3>
            <p class="fed-step-copy">
              Сколько трафика в месяц вы готовы отдать. Не гигабайтом больше: как только лимит выбран, нода сама выходит
              из выдачи и ждёт следующего месяца.
            </p>
          </article>
          <article class="fed-step">
            <img src="/img/oneui/terminal.svg" alt="" class="fed-step-icon" aria-hidden="true" />
            <h3 class="fed-step-title">Одна команда на сервере</h3>
            <p class="fed-step-copy">
              Установщик сам поставит агента, поднимет Xray и подключит ноду к пулу. Ничего настраивать руками не надо,
              и ваши собственные клиенты на этом сервере продолжают работать как работали.
            </p>
          </article>
          <article class="fed-step">
            <img src="/img/oneui/monitor.svg" alt="" class="fed-step-icon" aria-hidden="true" />
            <h3 class="fed-step-title">Дальше он работает сам</h3>
            <p class="fed-step-copy">
              Балансировщик раздаёт ноду бесплатным пользователям, следит за качеством и снимает её с выдачи, если
              сервер начал тупить. Вы видите только цифры: трафик, сессии, аптайм.
            </p>
          </article>
        </div>
      </section>

      <section class="surface-card mt-6">
        <h2 class="hero-title">Честно о том, что видно</h2>
        <p class="body-copy body-copy-wide mt-3">
          Про приватность принято молчать общими словами. Здесь сказано прямо, в обе стороны.
        </p>
        <div class="fed-honesty">
          <div class="surface-inset">
            <span class="section-kicker">Что видите вы</span>
            <p class="fed-step-copy mt-2">
              Только общие цифры: сколько трафика ушло, сколько было подключений, сколько времени сервер проработал.
              Кнопки, которая показала бы, кто именно им пользовался, в вашей панели нет - и это не выключенная
              настройка, такой возможности просто не существует.
            </p>
          </div>
          <div class="surface-inset">
            <span class="section-kicker">Что знает сервер</span>
            <p class="fed-step-copy mt-2">
              Ровно то же, что знает любой VPN-оператор о своём трафике. Поэтому на ноду не уходит никакой личности:
              профиль это случайный идентификатор, без почты, имени и привязки к аккаунту.
            </p>
          </div>
        </div>
      </section>

      <section class="surface-card mt-6">
        <h2 class="hero-title">Отдать сервер</h2>
        <p class="body-copy body-copy-wide mt-3">
          Нужен любой VPS с ядром 5.6 и свежее. Токен для установщика выдаётся в панели, в разделе
          <span class="wordmark-inline">Федерация</span>: там же видно, сколько ваши ноды уже отдали.
        </p>
        <div class="actions-row">
          <SamsungButton :to="{ name: 'login' }">Войти и получить токен</SamsungButton>
          <SamsungButton variant="ghost" :to="{ name: 'register' }">Завести аккаунт</SamsungButton>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup>
import SamsungButton from '@/components/layout/SamsungButton.vue';
import FederationCounters from '@/components/domain/FederationCounters.vue';
</script>

<style scoped>
.headline-dot {
  display: inline-block;
  width: 0.22em;
  height: 0.22em;
  margin-left: 0.12em;
  vertical-align: baseline;
}

.fed-hero-kicker {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-size: clamp(17px, 1.6vw, 20px);
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: rgba(252, 252, 252, 0.86);
}

/* Wordmark набран другим шрифтом и своим трекингом, из-за чего на общей строке
   он вставал выше и жирнее соседних слов. Здесь он подчиняется строке. */
.fed-hero-kicker .wordmark-inline {
  font-size: inherit;
  line-height: inherit;
  letter-spacing: inherit;
  color: inherit;
}

.fed-steps {
  margin-top: 20px;
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
}

.fed-step {
  border-radius: 20px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: #242427;
  padding: 20px;
}

.fed-step-icon {
  width: 44px;
  height: 44px;
  object-fit: contain;
}

.fed-step-title {
  margin-top: 14px;
  font-family: 'SamsungSharpSans', 'SamsungOne', sans-serif;
  font-size: 18px;
  font-weight: 700;
  color: #fbfbfb;
}

.fed-step-copy {
  margin-top: 8px;
  font-size: 14px;
  line-height: 1.55;
  color: rgba(252, 252, 252, 0.62);
}

.fed-honesty {
  margin-top: 20px;
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
}
</style>
