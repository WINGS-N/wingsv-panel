package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"v.wingsnet.org/internal/storage"
)

// microUSDT - миллионные доли USDT. Деньги ходят целыми числами: округли их
// один раз во float, и у кого-то потеряется копейка, о которой никто не узнает
type payoutEpochView struct {
	Number        uint64 `json:"number"`
	StartUnix     int64  `json:"start_unix"`
	EndUnix       int64  `json:"end_unix"`
	AmountMicro   uint64 `json:"amount_micro"`
	RootHex       string `json:"root_hex"`
	TxRef         string `json:"tx_ref"`
	PublishedUnix int64  `json:"published_unix"`
	// PayoutTx - транзакция, которой донору заплатили. Пустая означает, что
	// начислено, а деньги ещё не ушли: цифра без транзакции это обещание
	PayoutTx string `json:"payout_tx,omitempty"`
	PaidUnix int64  `json:"paid_unix,omitempty"`
	// MicroPerGiB - цена гигабайта в этой эпохе. Плавает по казне, без неё
	// выписка не объясняет, почему в этот раз меньше
	MicroPerGiB uint64 `json:"micro_per_gib,omitempty"`
}

// nodeAccrualView - что набежало одной машине и почему столько. Донору мало
// итога: он вправе видеть, какая нода сколько принесла и за что получила ноль
type nodeAccrualView struct {
	NodeID         string `json:"node_id"`
	Hostname       string `json:"hostname"`
	SelfBytes      uint64 `json:"self_bytes"`
	ReceiptBytes   uint64 `json:"receipt_bytes"`
	BillableBytes  uint64 `json:"billable_bytes"`
	ProbeConfirmed bool   `json:"probe_confirmed"`
	FactorBps      uint32 `json:"factor_bps"`
	AmountMicro    uint64 `json:"amount_micro"`
}

// payoutTermsView - прайс и границы периода
type payoutTermsView struct {
	MicroPerGiB     uint64 `json:"micro_per_gib"`
	PeriodSeconds   uint32 `json:"period_seconds"`
	PeriodStartUnix int64  `json:"period_start_unix"`
	PeriodEndUnix   int64  `json:"period_end_unix"`
}

type payoutStatementView struct {
	Enabled bool   `json:"enabled"`
	Error   string `json:"error,omitempty"`
	// Address - кошелёк, который донор назвал сам. Пустой означает, что
	// начисления копятся, но в эпоху он не попадает
	Address        string            `json:"address"`
	TotalMicro     uint64            `json:"total_micro"`
	ClaimableMicro uint64            `json:"claimable_micro"`
	Epochs         []payoutEpochView `json:"epochs"`
	// Cluster - в какой сети смотреть транзакции. Зашитый explorer однажды
	// покажет пустоту, потому что devnet и прод это разные миры
	Cluster string `json:"cluster,omitempty"`
	Terms          *payoutTermsView  `json:"terms,omitempty"`
	Pending        []nodeAccrualView `json:"pending"`
	PendingMicro   uint64            `json:"pending_micro"`
}

// handlePayoutStatement отвечает донору, сколько ему причитается и за что
func (h *Handler) handlePayoutStatement(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.federationOn() {
		writeJSON(w, http.StatusOK, payoutStatementView{Enabled: false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	got, err := h.fed.PayoutStatement(ctx, donorID(admin), uint32(limit))
	if err != nil {
		// Башка без выплат отвечает Unimplemented, и это не ошибка, а
		// состояние: раздел остаётся, цифр в нём просто нет
		writeJSON(w, http.StatusOK, payoutStatementView{
			Enabled: true,
			Error:   "выплаты пока не считаются: " + err.Error(),
		})
		return
	}
	view := payoutStatementView{
		Enabled:        true,
		Address:        got.GetAddress(),
		TotalMicro:     got.GetTotalMicro(),
		ClaimableMicro: got.GetClaimableMicro(),
	}
	for _, e := range got.GetEpochs() {
		view.Epochs = append(view.Epochs, payoutEpochView{
			Number: e.GetNumber(), StartUnix: e.GetStartUnix(), EndUnix: e.GetEndUnix(),
			AmountMicro: e.GetAmountMicro(), RootHex: e.GetRootHex(),
			TxRef: e.GetTxRef(), PublishedUnix: e.GetPublishedUnix(),
			PayoutTx: e.GetPayoutTx(), PaidUnix: e.GetPaidUnix(),
			MicroPerGiB: e.GetMicroPerGib(),
		})
	}
	if terms := got.GetTerms(); terms != nil {
		view.Terms = &payoutTermsView{
			MicroPerGiB: terms.GetMicroPerGib(), PeriodSeconds: terms.GetPeriodSeconds(),
			PeriodStartUnix: terms.GetPeriodStartUnix(), PeriodEndUnix: terms.GetPeriodEndUnix(),
		}
	}
	view.PendingMicro = got.GetPendingMicro()
	for _, n := range got.GetPending() {
		view.Pending = append(view.Pending, nodeAccrualView{
			NodeID: n.GetNodeId(), Hostname: n.GetHostname(),
			SelfBytes: n.GetSelfBytes(), ReceiptBytes: n.GetReceiptBytes(),
			BillableBytes: n.GetBillableBytes(), ProbeConfirmed: n.GetProbeConfirmed(),
			FactorBps: n.GetFactorBps(), AmountMicro: n.GetAmountMicro(),
		})
	}
	writeJSON(w, http.StatusOK, view)
}

type payoutAddressRequest struct {
	Address string `json:"address"`
}

// handlePayoutAddress записывает кошелёк донора. Проверку адреса делает башка:
// она же им и платит, и ошибиться тут значит отправить деньги в никуда
func (h *Handler) handlePayoutAddress(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.federationOn() {
		writeError(w, http.StatusNotFound, "федерация выключена")
		return
	}
	var req payoutAddressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "не разобрал запрос")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()
	if err := h.fed.SetPayoutAddress(ctx, donorID(admin), req.Address); err != nil {
		writeError(w, http.StatusBadGateway, "голова не приняла кошелёк: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"address": req.Address})
}
