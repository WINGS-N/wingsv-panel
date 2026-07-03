# High availability & stateless operation

The panel runs single-node by default (SQLite, one replica). This document
describes the stateless multi-replica model (milestone M6).

## Panel replicas

Run several panel replicas behind a load balancer with **no sticky sessions**.
State lives entirely outside the pods:

- **Shared database.** Set `DB_KIND=pgsql` and `DB_DSN` to a shared Postgres.
  Every replica reads and writes the same rows; Postgres has no single-writer
  limit, so a rolling update is safe.
- **Cross-replica bus.** A device's WebSocket and an admin's WebSocket can land
  on different replicas. The hub delivers to sinks it holds locally and, for a
  target on another replica, publishes over a bus so the holding replica
  delivers. On Postgres the bus rides `LISTEN/NOTIFY`: `Publish` inserts the
  payload into a `bus_messages` outbox and notifies with the row id (the NOTIFY
  payload is capped ~8 KB, so the id travels, not the bytes); every replica
  `LISTEN`s, reads the row and dispatches to its local handlers. A retention
  prune drops old rows. Admin fanout carries the origin replica id so the
  publisher does not double-deliver to its own local sinks.
  - `internal/bus` — the bus (`Postgres` and a `Nop` for single-node).
  - `internal/guardianhub` — `SendToClient`, `FanoutToAdmin`, `BroadcastToAdmins`
    publish cross-replica; `AttachBus` wires it.

Single-node SQLite / MariaDB deploys use the no-op bus and stay purely in-process
(there is only one replica, so nothing needs to cross).

### Presence and delivery

Client presence (`clients.online`) is written to the shared DB on connect and
disconnect, so any replica knows whether a device is online. `SendToClient`
delivers to a local sink if present; otherwise, when the client is online (on
another replica) it publishes to the bus; when offline it returns false and the
caller queues the command (`pending_commands`, drained on the client's next
connect to any replica) or reports the client offline.

## vk-turn-proxy relay HA

The relay is stateless per connection, so multiple relay IPs can sit behind a
DNS load balancer. Because WireGuard is connectionless, a client roams across
relay IPs only if every node presents the same peer set and the same server
keys:

- **Peer replication.** The panel owns peers in its DB and, on first resolve,
  replicates each client's peer (same public key and tunnel address) to every
  `vk_turn_proxy` node (`internal/provisioning` `replicatePeer`). A node that is
  down is skipped and picked up on the next resolve.
- **Shared server keys (deploy-time).** Roaming requires all nodes to run the
  same WireGuard server keypair, so the single config the client holds
  (`server_public_key` + endpoint) is valid on whichever node it reaches. Deploy
  the nodes with a shared server keypair; the panel does not mint server keys.

## Kubernetes

- `k8s/04-app.yml` — single-node default (SQLite PVC, `Recreate`, 1 replica).
- `k8s/04-app-ha.yml` — stateless multi-replica (`RollingUpdate`, 2+ replicas,
  no PVC). Point the config map at a shared Postgres. For SPKI-pin deploys, keep
  the CA in a shared Secret (cert-manager SelfSigned -> CA Issuer) so every
  replica serves the same pin.
