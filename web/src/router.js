import { createRouter, createWebHistory } from 'vue-router';

import LandingView from './views/LandingView.vue';
import FederationLanding from './views/FederationLanding.vue';
import LoginView from './views/LoginView.vue';
import RegisterView from './views/RegisterView.vue';
import AdminLayout from './views/admin/AdminLayout.vue';
import AdminClientList from './views/admin/ClientList.vue';
import AdminClientDetail from './views/admin/ClientDetail.vue';
import AdminAccount from './views/admin/AccountView.vue';
import AdminMasterSettings from './views/admin/MasterSettings.vue';
import AdminNodes from './views/admin/AdminNodes.vue';
import AdminFederation from '@/views/admin/AdminFederation.vue';
import AdminInvites from '@/views/admin/AdminInvites.vue';
import WgPeers from './views/shared/WgPeers.vue';
import NodeDetail from './views/shared/NodeDetail.vue';
import OwnerLayout from './views/owner/OwnerLayout.vue';
import OwnerOverview from './views/owner/OwnerOverview.vue';
import OwnerNodes from './views/owner/OwnerNodes.vue';
import OwnerAdmins from './views/owner/OwnerAdmins.vue';
import OwnerInviteTree from '@/views/owner/OwnerInviteTree.vue';
import CabinetLayout from '@/views/cabinet/CabinetLayout.vue';
import CabinetAccess from '@/views/cabinet/CabinetAccess.vue';
import CabinetDonate from '@/views/cabinet/CabinetDonate.vue';
import MatrixLanding from '@/views/MatrixLanding.vue';
import OwnerOracle from '@/views/owner/OwnerOracle.vue';
import OwnerOracleSubject from '@/views/owner/OwnerOracleSubject.vue';
import OwnerPayouts from '@/views/owner/OwnerPayouts.vue';
import OwnerUpstreams from '@/views/owner/OwnerUpstreams.vue';
import OwnerProbes from '@/views/owner/OwnerProbes.vue';
import OwnerFleet from '@/views/owner/OwnerFleet.vue';
import OwnerClients from './views/owner/OwnerClients.vue';
import OwnerAudit from './views/owner/OwnerAudit.vue';
import { authState, refreshSession, refreshRegistrationStatus, registrationState } from './stores/auth.js';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: LandingView, name: 'landing' },
    {
      path: '/me',
      component: CabinetLayout,
      children: [
        { path: '', component: CabinetAccess, name: 'cabinet-access' },
        { path: 'invites', component: AdminInvites, name: 'cabinet-invites' },
        { path: 'donate', component: CabinetDonate, name: 'cabinet-donate' },
        { path: 'account', component: AdminAccount, name: 'cabinet-account' },
      ],
    },
    { path: '/matrix', component: MatrixLanding, name: 'matrix-landing' },
    { path: '/federation', component: FederationLanding, name: 'federation-landing' },
    { path: '/login', component: LoginView, name: 'login' },
    { path: '/register', component: RegisterView, name: 'register' },
    {
      path: '/admin',
      component: AdminLayout,
      children: [
        { path: '', redirect: '/admin/clients' },
        { path: 'clients', component: AdminClientList, name: 'admin-clients' },
        {
          path: 'clients/:id/:tab?',
          component: AdminClientDetail,
          name: 'admin-client-detail',
          props: true,
        },
        { path: 'account', component: AdminAccount, name: 'admin-account' },
        { path: 'master', component: AdminMasterSettings, name: 'admin-master' },
        { path: 'nodes', component: AdminNodes, name: 'admin-nodes' },
        { path: 'federation', component: AdminFederation, name: 'admin-federation' },
        { path: 'invites', component: AdminInvites, name: 'admin-invites' },
        {
          path: 'nodes/:id',
          component: NodeDetail,
          name: 'admin-node-detail',
          props: { apiBase: '/api/admin', backName: 'admin-nodes' },
        },
        {
          path: 'wgpeers',
          component: WgPeers,
          name: 'admin-wgpeers',
          props: { apiBase: '/api/admin' },
        },
      ],
    },
    {
      path: '/owner',
      component: OwnerLayout,
      children: [
        { path: '', redirect: '/owner/overview' },
        { path: 'overview', component: OwnerOverview, name: 'owner-overview' },
        { path: 'nodes', component: OwnerNodes, name: 'owner-nodes' },
        {
          path: 'nodes/:id',
          component: NodeDetail,
          name: 'owner-node-detail',
          props: { apiBase: '/api/owner', backName: 'owner-nodes' },
        },
        {
          path: 'wgpeers',
          component: WgPeers,
          name: 'owner-wgpeers',
          props: { apiBase: '/api/owner' },
        },
        { path: 'admins', component: OwnerAdmins, name: 'owner-admins' },
        { path: 'invite-tree', component: OwnerInviteTree, name: 'owner-invite-tree' },
        { path: 'fleet', component: OwnerFleet, name: 'owner-fleet' },
        { path: 'clients', component: OwnerClients, name: 'owner-clients' },
        { path: 'probes', component: OwnerProbes, name: 'owner-probes' },
        { path: 'oracle', component: OwnerOracle, name: 'owner-oracle' },
        { path: 'oracle/:id', component: OwnerOracleSubject, name: 'owner-oracle-subject' },
        { path: 'payouts', component: OwnerPayouts, name: 'owner-payouts' },
        { path: 'upstreams', component: OwnerUpstreams, name: 'owner-upstreams' },
        { path: 'audit', component: OwnerAudit, name: 'owner-audit' },
      ],
    },
  ],
});

// Владелец панель не теряет никогда, поэтому его роль важнее флага
function hasPanel(admin) {
  return Boolean(admin && (admin.panel_access || admin.role === 'owner'));
}

function homeFor(admin) {
  return hasPanel(admin) ? '/admin/clients' : '/me';
}

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
  if (to.path.startsWith('/admin') || to.path.startsWith('/owner') || to.path.startsWith('/me')) {
    if (!authState.value.admin) {
      await refreshSession();
    }
    if (!authState.value.admin) {
      return { path: '/login', query: { redirect: to.fullPath } };
    }
  }
  // Аккаунт без доступа в панель - обычный участник: ему свой кабинет, а не
  // чужие ноды и клиенты
  if (to.path.startsWith('/admin') && authState.value.admin && !hasPanel(authState.value.admin)) {
    return { path: '/me' };
  }
  if (to.path.startsWith('/owner') && authState.value.admin?.role !== 'owner') {
    return { path: homeFor(authState.value.admin) };
  }
  if (to.name === 'login' && authState.value.admin) {
    return { path: homeFor(authState.value.admin) };
  }
  if (to.name === 'register') {
    if (authState.value.admin) {
      return { path: homeFor(authState.value.admin) };
    }
    if (registrationState.value.mode === 'closed') {
      return { path: '/login' };
    }
  }
  return true;
});

export default router;
