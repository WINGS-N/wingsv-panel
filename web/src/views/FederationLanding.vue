<template>
  <!-- ============================================================
       ПРОПАГАНДА ФЕДЕРАЦИИ - отдельная страница, на которую ведут
       из приложения и из панели. Задача одна: объяснить, зачем
       отдавать свой сервер, и показать, что пул живой прямо сейчас.
       ============================================================ -->
  <div class="landing-stage">
    <PublicTopbar tag="Federation" />

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
            Во благо народа.
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
          <img src="/img/oneui/sharing.svg" alt="" class="landing-hero-illustration" />
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
        <h2 class="hero-title">Безопасность</h2>
        <p class="oracle-line">
          Secured by <span class="wordmark-inline">WINGS V</span>
          <span class="oracle-divider" aria-hidden="true">|</span>
          <span class="oracle-name">Oracle</span>
        </p>
        <p class="body-copy body-copy-wide mt-3">
          У каждого аккаунта есть уровень доверия. Он начинается посередине и двигается сам: подозрительное поведение
          опускает его, спокойное - поднимает обратно, потому что вес каждого сигнала со временем тает. Высокий уровень
          - несколько серверов на выбор, средний - один и с урезанной скоростью, низкий - карантин. Каждое решение
          записывается вместе с тем, что на него повлияло, чтобы на вопрос "за что" был ответ, а не пожатие плечами.
        </p>
        <div class="fed-rail">
          <button type="button" class="fed-rail-nav is-prev" aria-label="Предыдущие карточки" @click="scrollRail(-1)">
            <ChevronLeft :size="18" aria-hidden="true" />
          </button>
          <div ref="rail" class="fed-rail-track">
            <article class="fed-step">
              <img src="/img/oneui/firewall.svg" alt="" class="fed-step-icon" aria-hidden="true" />
              <h3 class="fed-step-title">Вход только по приглашению</h3>
              <p class="fed-step-copy">
                Аккаунты не раздаются всем подряд: каждого кто-то привёл, и это записано. Ветку можно срезать целиком -
                вместе с тем, кто наприглашал, уходят все, кого привёл он. Заводить аккаунты пачками становится дорого,
                потому что теряются они тоже пачками.
              </p>
            </article>
            <article class="fed-step">
              <img src="/img/oneui/monitor.svg" alt="" class="fed-step-icon" aria-hidden="true" />
              <h3 class="fed-step-title">Один аккаунт - один человек</h3>
              <p class="fed-step-copy">
                Своим доступом человек пользуется с двух-трёх устройств. Если один и тот же доступ работает сразу из
                десятка разных мест, это уже не человек, а перепродажа - и такой аккаунт уходит в карантин. Чтобы это
                увидеть, достаточно посчитать, откуда заходят; что именно открывают, знать не нужно.
              </p>
            </article>
            <article class="fed-step">
              <img src="/img/oneui/privacy.svg" alt="" class="fed-step-icon" aria-hidden="true" />
              <h3 class="fed-step-title">Спам и торренты режутся на узле</h3>
              <p class="fed-step-copy">
                Почтовые порты и bittorrent закрыты на самой ноде - не потому, что мы против, а потому, что жалоба
                хостеру прилетит донору, отдавшему сервер. Он подписывался делиться трафиком, а не отвечать за чужую
                рассылку.
              </p>
            </article>
            <article class="fed-step">
              <img src="/img/oneui/server.svg" alt="" class="fed-step-icon" aria-hidden="true" />
              <h3 class="fed-step-title">Ключи не уезжают с сервера</h3>
              <p class="fed-step-copy">
                Входную личность узла генерирует сам узел, наружу отдаётся только открытая половина. Взломанный сервер
                уносит с собой свои ключи и ничьи больше - одной утечки не хватит, чтобы прикинуться остальными.
              </p>
            </article>
            <article class="fed-step">
              <img src="/img/oneui/sharing.svg" alt="" class="fed-step-icon" aria-hidden="true" />
              <h3 class="fed-step-title">Узел не знает, кто через него идёт</h3>
              <p class="fed-step-copy">
                Профиль приезжает на сервер случайной строкой: ни почты, ни имени, ни номера. Донор видит трафик и
                количество сессий, а кнопки, которая превратила бы их в людей, нет ни у него, ни у нас.
              </p>
            </article>
            <article class="fed-step">
              <img src="/img/oneui/terminal.svg" alt="" class="fed-step-icon" aria-hidden="true" />
              <h3 class="fed-step-title">Узлы проверяются оттуда, где ими пользуются</h3>
              <p class="fed-step-copy">
                Сервер, доступный из Европы, может быть придушен для домашнего провайдера. Поэтому качество меряется не
                пингом из дата-центра, а реальной скачкой с точек внутри страны - и сервер, через который перестало
                течь, уходит из выдачи сам.
              </p>
            </article>
          </div>
          <button type="button" class="fed-rail-nav is-next" aria-label="Следующие карточки" @click="scrollRail(1)">
            <ChevronRight :size="18" aria-hidden="true" />
          </button>
        </div>
        <p class="body-copy body-copy-wide mt-4">
          Сейчас всё это считается по количеству и поведению, а не по содержимому. Обещать, что так будет всегда, мы не
          станем: если сервис начнут использовать для рассылки или мошенничества, разбираться придётся предметно. Что
          останется неизменным - разбор нужен, чтобы отрезать нарушителя, а не чтобы собирать историю посещений.
        </p>
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
import { ref } from 'vue';
import { ChevronLeft, ChevronRight } from 'lucide-vue-next';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import PublicTopbar from '@/components/layout/PublicTopbar.vue';
import FederationCounters from '@/components/domain/FederationCounters.vue';

const rail = ref(null);

// Листаем на ширину видимой части минус карточка, чтобы соседняя оставалась в
// кадре и было видно, что список продолжается
function scrollRail(direction) {
  const el = rail.value;
  if (!el) return;
  el.scrollBy({ left: direction * Math.max(240, el.clientWidth - 120), behavior: 'smooth' });
}
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

.fed-rail {
  position: relative;
  margin-top: 20px;
}

.fed-rail-track {
  display: flex;
  gap: 16px;
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  scrollbar-width: none;
  padding-bottom: 4px;
}

.fed-rail-track::-webkit-scrollbar {
  display: none;
}

.fed-rail-track > .fed-step {
  flex: 0 0 min(300px, 78vw);
  scroll-snap-align: start;
}

.fed-rail-nav {
  position: absolute;
  top: 50%;
  z-index: 2;
  display: none;
  transform: translateY(-50%);
  width: 36px;
  height: 36px;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(20, 20, 22, 0.92);
  color: #fbfbfb;
  transition: background 0.15s ease;
}

.fed-rail-nav:hover {
  background: rgba(40, 40, 44, 0.96);
}

.fed-rail-nav.is-prev {
  left: -14px;
}

.fed-rail-nav.is-next {
  right: -14px;
}

@media (min-width: 768px) {
  .fed-rail-nav {
    display: flex;
  }
}

.fed-honesty {
  margin-top: 20px;
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
}
</style>
