// Lightweight admin /api/admin/ws helper. Each consumer registers a handler
// callback; the socket is shared across consumers via a tiny ref-count.

let socket = null;
let listeners = new Set();
let reconnectTimer = null;

function ensureSocket() {
  if (socket && socket.readyState <= 1) return socket;
  const wsUrl = `${location.origin.replace(/^http/, "ws")}/api/admin/ws`;
  socket = new WebSocket(wsUrl);
  socket.addEventListener("message", (event) => {
    let payload;
    try {
      payload = JSON.parse(event.data);
    } catch {
      return;
    }
    for (const fn of listeners) {
      try { fn(payload); } catch (err) { console.warn("admin-ws listener", err); }
    }
  });
  socket.addEventListener("close", () => {
    socket = null;
    if (listeners.size > 0 && !reconnectTimer) {
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        ensureSocket();
      }, 5000);
    }
  });
  socket.addEventListener("error", () => {
    if (socket && socket.readyState === WebSocket.OPEN) socket.close();
  });
  return socket;
}

export function connectAdminSocket(handler) {
  listeners.add(handler);
  ensureSocket();
  return {
    close() {
      listeners.delete(handler);
      if (listeners.size === 0 && socket) {
        socket.close();
        socket = null;
      }
    },
  };
}
