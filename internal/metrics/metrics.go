// Package metrics exposes panel and vk-turn-proxy node stats as Prometheus
// metrics. Per-node gauges (peers, active sessions, aggregate traffic) are
// pulled from each registered vk-turn-proxy over its Relay gRPC API at scrape
// time.
package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"v.wingsnet.org/internal/relayclient"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

// NodeStatser fetches a relay node's live status. relayclient.Provisioner
// implements it.
type NodeStatser interface {
	NodeStatus(ctx context.Context, node dbmodel.ServerNode) (relayclient.RelayStatus, error)
}

// Hub reports live WebSocket connection counts.
type Hub interface {
	ClientCount() int
	AdminCount() int
}

// Collector implements prometheus.Collector.
type Collector struct {
	store   *storage.Store
	hub     Hub
	nodes   NodeStatser
	timeout time.Duration

	clientsTotal  *prometheus.Desc
	clientsOnline *prometheus.Desc
	adminsTotal   *prometheus.Desc
	wsClients     *prometheus.Desc
	wsAdmins      *prometheus.Desc
	serverNodes   *prometheus.Desc
	nodeUp        *prometheus.Desc
	nodePeers     *prometheus.Desc
	nodeSessions  *prometheus.Desc
	nodeRx        *prometheus.Desc
	nodeTx        *prometheus.Desc
}

func NewCollector(store *storage.Store, hub Hub, nodes NodeStatser) *Collector {
	d := func(name, help string, labels ...string) *prometheus.Desc {
		return prometheus.NewDesc(name, help, labels, nil)
	}
	return &Collector{
		store:         store,
		hub:           hub,
		nodes:         nodes,
		timeout:       3 * time.Second,
		clientsTotal:  d("wingsv_clients_total", "Total managed clients."),
		clientsOnline: d("wingsv_clients_online", "Clients currently online."),
		adminsTotal:   d("wingsv_admins_total", "Total admins."),
		wsClients:     d("wingsv_ws_clients", "Live device WebSocket connections."),
		wsAdmins:      d("wingsv_ws_admins", "Live admin WebSocket connections."),
		serverNodes:   d("wingsv_server_nodes", "Registered backend nodes.", "kind"),
		nodeUp:        d("wingsv_vkturn_up", "1 if the vk-turn-proxy node answered its Relay API.", "node"),
		nodePeers:     d("wingsv_vkturn_peers", "WireGuard peers programmed on the node.", "node"),
		nodeSessions:  d("wingsv_vkturn_active_sessions", "Active relay sessions on the node.", "node"),
		nodeRx:        d("wingsv_vkturn_rx_bytes", "Aggregate peer RX bytes on the node.", "node"),
		nodeTx:        d("wingsv_vkturn_tx_bytes", "Aggregate peer TX bytes on the node.", "node"),
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{
		c.clientsTotal, c.clientsOnline, c.adminsTotal, c.wsClients, c.wsAdmins,
		c.serverNodes, c.nodeUp, c.nodePeers, c.nodeSessions, c.nodeRx, c.nodeTx,
	} {
		ch <- desc
	}
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	gauge := func(desc *prometheus.Desc, v float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, labels...)
	}

	if counts, err := c.store.CountClients(); err == nil {
		gauge(c.clientsTotal, float64(counts.Total))
		gauge(c.clientsOnline, float64(counts.Online))
	}
	if n, err := c.store.CountAdmins(); err == nil {
		gauge(c.adminsTotal, float64(n))
	}
	if c.hub != nil {
		gauge(c.wsClients, float64(c.hub.ClientCount()))
		gauge(c.wsAdmins, float64(c.hub.AdminCount()))
	}

	nodes, err := c.store.ListServerNodes("")
	if err != nil {
		return
	}
	byKind := map[string]int{}
	for _, node := range nodes {
		byKind[node.Kind]++
	}
	for kind, count := range byKind {
		gauge(c.serverNodes, float64(count), kind)
	}
	if c.nodes == nil {
		return
	}
	for _, node := range nodes {
		if node.Kind != storage.ServerNodeVKTurnProxy {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		st, statErr := c.nodes.NodeStatus(ctx, node)
		cancel()
		if statErr != nil {
			gauge(c.nodeUp, 0, node.ID)
			continue
		}
		gauge(c.nodeUp, 1, node.ID)
		gauge(c.nodePeers, float64(st.PeerCount), node.ID)
		gauge(c.nodeSessions, float64(st.ActiveSessions), node.ID)
		ch <- prometheus.MustNewConstMetric(c.nodeRx, prometheus.CounterValue, float64(st.RxBytes), node.ID)
		ch <- prometheus.MustNewConstMetric(c.nodeTx, prometheus.CounterValue, float64(st.TxBytes), node.ID)
	}
}

// Handler returns an HTTP handler exposing the collector's metrics.
func Handler(c *Collector) http.Handler {
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}
