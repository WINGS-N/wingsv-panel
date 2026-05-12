import { computed, ref } from "vue";

export const authState = ref({ admin: null, loading: false });
export const registrationState = ref({ mode: "open", loaded: false });

export const isOwner = computed(() => authState.value.admin?.role === "owner");

export function avatarUrlFor(admin) {
  if (!admin || !admin.id) return "/img/avatar-default.png";
  if (admin.avatar_version && admin.avatar_version > 0) {
    return `/api/admin/avatars/${admin.id}.png?v=${admin.avatar_version}`;
  }
  return "/img/avatar-default.png";
}

export const myAvatarUrl = computed(() => avatarUrlFor(authState.value.admin));

export async function refreshSession() {
  authState.value.loading = true;
  try {
    const res = await fetch("/api/admin/me", { credentials: "include" });
    if (res.ok) {
      const data = await res.json();
      authState.value.admin = data;
    } else {
      authState.value.admin = null;
    }
  } catch (error) {
    authState.value.admin = null;
  } finally {
    authState.value.loading = false;
  }
}

export async function refreshRegistrationStatus() {
  try {
    const res = await fetch("/api/admin/registration-status");
    if (res.ok) {
      const data = await res.json();
      registrationState.value = { mode: data.mode || "open", loaded: true };
    }
  } catch {
    // Keep last-known mode on transient failure.
  }
}

export async function login(username, password) {
  const res = await fetch("/api/admin/login", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.message || "login failed");
  }
  const data = await res.json();
  authState.value.admin = data;
  return data;
}

export async function register({ username, password, inviteToken }) {
  const res = await fetch("/api/admin/register", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      username,
      password,
      invite_token: inviteToken || "",
    }),
  });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.message || "register failed");
  }
  const data = await res.json();
  authState.value.admin = data;
  return data;
}

export async function logout() {
  await fetch("/api/admin/logout", {
    method: "POST",
    credentials: "include",
  });
  authState.value.admin = null;
}

export async function changePassword(oldPassword, newPassword) {
  const res = await fetch("/api/admin/password", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
  });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.message || "change password failed");
  }
  if (authState.value.admin) {
    authState.value.admin.must_change_password = false;
  }
}
