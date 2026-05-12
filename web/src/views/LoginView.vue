<template>
  <!-- ============================================================
       LOGIN — centered Samsung Account-style dialog card.
       Two big-typography lines as headline, sub-line in muted text,
       underline inputs, solid-blue pill primary, neutral secondary.
       ============================================================ -->
  <div class="login-stage">
    <header class="samsung-topbar">
      <router-link class="samsung-topbar-brand" to="/">
        <span class="wordmark-inline">WINGS V</span>
        <span class="samsung-topbar-divider">|</span>
        <span class="samsung-topbar-tag">Control Panel</span>
      </router-link>
    </header>

    <main class="login-main">
      <section class="login-card surface-card">
        <h1 class="login-headline">
          <span>Один аккаунт.</span>
          <span>Любое устройство.</span>
          <span>Только для вас.</span>
        </h1>
        <p class="login-sub">Войдите для начала</p>

        <form class="login-form" @submit.prevent="onSubmit">
          <div class="input-field">
            <OneuiInput
              v-model.trim="username"
              label="Логин"
              autocomplete="username"
            />
          </div>

          <div class="input-field">
            <OneuiInput
              v-model="password"
              label="Пароль"
              type="password"
              autocomplete="current-password"
            />
          </div>

          <p v-if="error" class="state-error">{{ error }}</p>

          <SamsungButton
            class="login-submit"
            type="submit"
            :busy="busy"
            :disabled="!username || !password"
          >
            <template #icon><LogIn class="button-icon" aria-hidden="true" /></template>
            {{ busy ? "Входим…" : "Войти" }}
          </SamsungButton>

          <router-link
            v-if="registrationState.mode !== 'closed'"
            class="login-back-link"
            :to="{ name: 'register' }"
          >Создать аккаунт</router-link>
        </form>
      </section>
    </main>

    <footer class="login-footer">
      <span class="wordmark-inline">WINGS V</span>
      <span class="login-footer-meta">WINGS-N · {{ year }} · All rights reserved</span>
    </footer>
  </div>
</template>

<script setup>
import { computed, ref } from "vue";
import { useRouter, useRoute } from "vue-router";
import { LogIn } from "lucide-vue-next";
import { login, registrationState } from "@/stores/auth.js";
import OneuiInput from "@/components/controls/OneuiInput.vue";
import SamsungButton from "@/components/layout/SamsungButton.vue";

const router = useRouter();
const route = useRoute();
const username = ref("");
const password = ref("");
const error = ref("");
const busy = ref(false);
const year = computed(() => new Date().getFullYear());

async function onSubmit() {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    await login(username.value, password.value);
    const target = typeof route.query.redirect === "string" ? route.query.redirect : "/admin/clients";
    router.push(target);
  } catch (err) {
    error.value = err.message || "Не удалось войти";
  } finally {
    busy.value = false;
  }
}
</script>

<style scoped>
.login-stage {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.login-main {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 24px;
}

.login-card {
  width: min(560px, 100%);
  padding: 56px 56px 64px;
}

@media (max-width: 640px) {
  .login-card {
    padding: 32px 22px 40px;
    border-radius: 22px;
  }
}

.login-headline {
  text-align: center;
  font-family: "SamsungSharpSans", "SamsungOne", sans-serif;
  font-weight: 700;
  font-size: clamp(22px, 2.8vw, 28px);
  line-height: 1.25;
  letter-spacing: -0.005em;
  color: #fbfbfb;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.login-sub {
  margin: 14px 0 0;
  text-align: center;
  font-size: 15px;
  color: rgba(252, 252, 252, 0.62);
}

.login-form {
  margin-top: 56px;
  display: flex;
  flex-direction: column;
  gap: 28px;
}

.login-submit {
  margin-top: 8px;
  width: 100%;
}

.login-back-link {
  margin-top: 6px;
  text-align: center;
  font-size: 14px;
  color: rgba(252, 252, 252, 0.78);
  text-decoration: underline;
  text-underline-offset: 3px;
}

.login-back-link:hover {
  color: #fbfbfb;
}

.login-footer {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 12px;
  padding: 24px 40px 32px;
  font-family: "SamsungSharpSans", "SamsungOne", sans-serif;
  font-size: 18px;
  color: rgba(252, 252, 252, 0.4);
}

.login-footer-meta {
  font-family: "SamsungOne", sans-serif;
  font-size: 12px;
}

@media (max-width: 640px) {
  .login-footer {
    padding: 18px 20px 24px;
    font-size: 16px;
  }
}
</style>
