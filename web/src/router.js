import { createRouter, createWebHistory } from "vue-router";

import LandingView from "./views/LandingView.vue";
import LoginView from "./views/LoginView.vue";
import RegisterView from "./views/RegisterView.vue";
import AdminLayout from "./views/admin/AdminLayout.vue";
import AdminClientList from "./views/admin/ClientList.vue";
import AdminClientDetail from "./views/admin/ClientDetail.vue";
import AdminAccount from "./views/admin/AccountView.vue";
import AdminMasterSettings from "./views/admin/MasterSettings.vue";
import OwnerLayout from "./views/owner/OwnerLayout.vue";
import OwnerOverview from "./views/owner/OwnerOverview.vue";
import OwnerAdmins from "./views/owner/OwnerAdmins.vue";
import OwnerClients from "./views/owner/OwnerClients.vue";
import OwnerAudit from "./views/owner/OwnerAudit.vue";
import { authState, refreshSession, refreshRegistrationStatus, registrationState } from "./stores/auth.js";

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", component: LandingView, name: "landing" },
    { path: "/login", component: LoginView, name: "login" },
    { path: "/register", component: RegisterView, name: "register" },
    {
      path: "/admin",
      component: AdminLayout,
      children: [
        { path: "", redirect: "/admin/clients" },
        { path: "clients", component: AdminClientList, name: "admin-clients" },
        {
          path: "clients/:id/:tab?",
          component: AdminClientDetail,
          name: "admin-client-detail",
          props: true,
        },
        { path: "account", component: AdminAccount, name: "admin-account" },
        { path: "master", component: AdminMasterSettings, name: "admin-master" },
      ],
    },
    {
      path: "/owner",
      component: OwnerLayout,
      children: [
        { path: "", redirect: "/owner/overview" },
        { path: "overview", component: OwnerOverview, name: "owner-overview" },
        { path: "admins", component: OwnerAdmins, name: "owner-admins" },
        { path: "clients", component: OwnerClients, name: "owner-clients" },
        { path: "audit", component: OwnerAudit, name: "owner-audit" },
      ],
    },
  ],
});

let sessionProbed = false;

router.beforeEach(async (to) => {
  if (!registrationState.value.loaded) {
    await refreshRegistrationStatus();
  }
  // Probe the session cookie at least once so /login and /register can
  // redirect already-authenticated users without forcing them to type the
  // password again.
  if (!sessionProbed && !authState.value.admin) {
    await refreshSession();
    sessionProbed = true;
  }
  if (to.path.startsWith("/admin") || to.path.startsWith("/owner")) {
    if (!authState.value.admin) {
      await refreshSession();
    }
    if (!authState.value.admin) {
      return { path: "/login", query: { redirect: to.fullPath } };
    }
  }
  if (to.path.startsWith("/owner") && authState.value.admin?.role !== "owner") {
    return { path: "/admin/clients" };
  }
  if (to.name === "login" && authState.value.admin) {
    return { path: "/admin/clients" };
  }
  if (to.name === "register") {
    if (authState.value.admin) {
      return { path: "/admin/clients" };
    }
    if (registrationState.value.mode === "closed") {
      return { path: "/login" };
    }
  }
  return true;
});

export default router;
