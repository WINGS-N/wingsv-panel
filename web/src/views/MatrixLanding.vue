<template>
  <div class="landing-shell">
    <PublicTopbar tag="Federation" />

    <main class="landing-main">
      <section class="surface-card">
        <h1 class="hero-title">Вход по Matrix ID</h1>
        <p class="body-copy body-copy-wide">
          Matrix - открытый протокол переписки, где аккаунт живёт не у одной компании, а на сервере, который выбираете
          вы. Наш сервер выдаёт такой аккаунт, и им же открывается вход сюда: отдельный пароль заводить не нужно.
        </p>

        <div class="fed-steps">
          <article class="fed-step">
            <img src="/img/oneui/privacy.svg" alt="" class="fed-step-icon" aria-hidden="true" />
            <h3 class="fed-step-title">Один аккаунт на всё</h3>
            <p class="fed-step-copy">
              Тот же Matrix ID открывает переписку и панель. Пароль хранится на сервере аккаунтов, сюда он не попадает
              вообще - мы получаем только подтверждение, что вход состоялся.
            </p>
          </article>
          <article class="fed-step">
            <img src="/img/oneui/server.svg" alt="" class="fed-step-icon" aria-hidden="true" />
            <h3 class="fed-step-title">Только наш сервер</h3>
            <p class="fed-step-copy">
              Пускаем аккаунты одного домена. Чужой Matrix ID зарегистрировать можно где угодно и сколько угодно, а
              вместе с ним обнулилась бы вся защита от ферм.
            </p>
          </article>
          <article class="fed-step">
            <img src="/img/oneui/firewall.svg" alt="" class="fed-step-icon" aria-hidden="true" />
            <h3 class="fed-step-title">Приглашение всё равно нужно</h3>
            <p class="fed-step-copy">
              Matrix ID - это способ войти, а не пропуск. Бесплатные серверы федерации открываются по коду приглашения,
              и без него аккаунт остаётся просто аккаунтом.
            </p>
          </article>
        </div>

        <div class="actions-row">
          <SamsungButton @click="goLogin">
            <template #icon><LogIn class="button-icon" aria-hidden="true" /></template>
            Войти
          </SamsungButton>
          <SamsungButton variant="ghost" @click="goRegister">Завести аккаунт</SamsungButton>
        </div>
      </section>

      <section class="surface-card mt-6">
        <h2 class="section-title">Как это выглядит</h2>
        <ol class="matrix-steps">
          <li>Нажимаете «Войти через Matrix ID» - открывается страница нашего сервера аккаунтов.</li>
          <li>Вводите там логин и пароль, при включённом втором факторе - код из приложения.</li>
          <li>Возвращаетесь обратно уже вошедшим. В приложении WINGS V возврат происходит сам.</li>
        </ol>
        <p class="admin-muted mt-3">
          Сервер аккаунтов: <span class="admin-mono">{{ homeserver || 'mxaccount.wingsnet.org' }}</span>
        </p>
      </section>
    </main>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { LogIn } from 'lucide-vue-next';
import PublicTopbar from '@/components/layout/PublicTopbar.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';

const router = useRouter();
const homeserver = ref('');
// Приглашение переживает переход: человек пришёл по ссылке и не должен терять
// код, читая объяснение
const invite = new URLSearchParams(window.location.search).get('invite') || '';

onMounted(async () => {
  try {
    const res = await fetch('/api/oidc/status');
    if (res.ok) {
      homeserver.value = (await res.json()).homeserver || '';
    }
  } catch {
    // Адрес сервера - справка, без него страница читается так же
  }
});

function goLogin() {
  router.push({ name: 'login', query: invite ? { invite } : {} });
}

function goRegister() {
  router.push({ name: 'register', query: invite ? { invite } : {} });
}
</script>
