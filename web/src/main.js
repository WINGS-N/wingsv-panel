import { createApp } from "vue";
import App from "./App.vue";
import router from "./router.js";
import "./stores/theme.js";
import "./styles.css";

createApp(App).use(router).mount("#app");

const bootLoader = document.getElementById("boot-loader");
if (bootLoader) {
  requestAnimationFrame(() => {
    bootLoader.classList.add("is-hidden");
    window.setTimeout(() => {
      bootLoader.remove();
    }, 240);
  });
}

if ("serviceWorker" in navigator) {
  window.addEventListener("load", async () => {
    try {
      await navigator.serviceWorker.register("/sw.js");
    } catch (error) {
      console.warn("service worker registration failed", error);
    }
  });
}
