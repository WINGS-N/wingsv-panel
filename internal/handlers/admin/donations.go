package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"v.wingsnet.org/internal/solana"
	"v.wingsnet.org/internal/storage"
)

// Адреса лежат в настройках площадки, а не в коде: кошелёк меняют, а гонять
// выкат ради одной строки это дурость. Адрес разработки прошит дефолтом -
// он уже есть, и менять его никто не собирается
const (
	settingTrafficWallet = "donation_traffic_address"
	settingDevWallet     = "donation_dev_address"
	settingDonationMint  = "donation_mint"
	// Аккаунт программы выплат, в котором лежит ставка оператора. Пусто -
	// раздел просто не показывает долю, а не ломается
	settingFeeState = "donation_fee_state"

	defaultDevWallet = "D7jqCxX6huU3tQJGTLSNPMx3sR4op4fbeq689rwm9A7g"
	// USDT на Solana. Занос другим токеном не засчитывается: считать курс мы не
	// умеем, да и не хотим
	defaultDonationMint = "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB"
)

type donationWalletsView struct {
	// Traffic - котёл, из которого платят донорам за отданный трафик
	Traffic string `json:"traffic"`
	// Dev - разработка. Эти деньги в котёл не идут и донорам не раздаются
	Dev  string `json:"dev"`
	Mint string `json:"mint"`
}

type donationView struct {
	Kind        string `json:"kind"`
	AmountMicro int64  `json:"amount_micro"`
	Reference   string `json:"reference"`
	AtUnix      int64  `json:"at_unix"`
}

type donationsResponse struct {
	Wallets donationWalletsView `json:"wallets"`
	// Memo - код, который человек кладёт в заметку перевода. Без него занос
	// прилетит анонимным, и привязать его будет не к кому
	Memo string         `json:"memo"`
	Mine []donationView `json:"mine"`
	// TotalMicro - сколько человек занёс всего, TrustCredit - на сколько очков
	// это греет доверие прямо сейчас
	TotalMicro  int64   `json:"total_micro"`
	TrustCredit float64 `json:"trust_credit"`
	// CutBps - сколько удерживает оператор с заноса на трафик. Цифра читается
	// из цепочки, потому что там она и решается
	CutBps uint16 `json:"cut_bps"`
}

func (h *Handler) donationWallets() donationWalletsView {
	traffic, _ := h.store.GetPlatformSetting(settingTrafficWallet, "")
	dev, _ := h.store.GetPlatformSetting(settingDevWallet, defaultDevWallet)
	mint, _ := h.store.GetPlatformSetting(settingDonationMint, defaultDonationMint)
	return donationWalletsView{Traffic: traffic, Dev: dev, Mint: mint}
}

// handleDonations отдаёт адреса и то, что человек уже занёс
func (h *Handler) handleDonations(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rows, err := h.store.DonationsOf(admin.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не смог поднять заносы")
		return
	}
	out := donationsResponse{Wallets: h.donationWallets(), Memo: MemoFor(admin.ID)}
	for _, row := range rows {
		out.Mine = append(out.Mine, donationView{
			Kind: row.Kind, AmountMicro: row.AmountMicro,
			Reference: row.Reference, AtUnix: row.At.Unix(),
		})
		out.TotalMicro += row.AmountMicro
	}
	out.TrustCredit = h.trustCredit(r.Context(), admin)
	out.CutBps = h.operatorCut(r.Context())
	writeJSON(w, http.StatusOK, out)
}

// trustCredit спрашивает башку, на сколько занос греет доверие. Молчит, если
// федерация выключена: раздел от этого не ломается
func (h *Handler) trustCredit(ctx context.Context, admin storage.Admin) float64 {
	if !h.federationOn() {
		return 0
	}
	call, cancel := context.WithTimeout(ctx, federationTimeout)
	defer cancel()
	credit, err := h.fed.ReportDonation(call, federationUserID(admin), 0, 0, "")
	if err != nil {
		return 0
	}
	return credit
}

type claimDonationRequest struct {
	Signature string `json:"signature"`
	Kind      string `json:"kind"`
}

// handleClaimDonation засчитывает занос по подписи транзакции.
//
// Верить на слово нельзя: за донат греется доверие, поэтому транзакция
// поднимается из цепочки и проверяется - тот ли адрес, тот ли токен, прошла ли
// она вообще
func (h *Handler) handleClaimDonation(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req claimDonationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "не разобрал запрос")
		return
	}
	signature := strings.TrimSpace(req.Signature)
	if signature == "" {
		writeError(w, http.StatusBadRequest, "не указана подпись транзакции")
		return
	}
	kind := "traffic"
	if req.Kind == "dev" {
		kind = "dev"
	}
	wallets := h.donationWallets()
	wallet := wallets.Traffic
	if kind == "dev" {
		wallet = wallets.Dev
	}
	if wallet == "" {
		writeError(w, http.StatusNotFound, "адрес для этого доната ещё не настроен")
		return
	}

	transfer, err := solana.New("").VerifyTransfer(r.Context(), signature, wallet, wallets.Mint)
	if err != nil {
		writeError(w, http.StatusBadRequest, donationError(err))
		return
	}
	// Txid публичен нахуй, и без этой проверки любой хитрожопый вписал бы себе
	// чужой занос, подсмотренный в эксплорере
	if id, ok := adminFromMemo(transfer.Memo); !ok || id != admin.ID {
		writeError(w, http.StatusBadRequest,
			"в заметке перевода нет вашего кода "+MemoFor(admin.ID)+", засчитать его вам нельзя")
		return
	}
	fresh, err := h.store.RecordDonation(admin.ID, kind, signature, int64(transfer.AmountMicro), transfer.At)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не смог записать занос")
		return
	}
	if !fresh {
		writeError(w, http.StatusConflict, "эта транзакция уже засчитана")
		return
	}

	// Доверие греет только общий котёл: разработка это нам на хлеб, и продавать
	// за неё отношение Oracle было бы наёбкой
	var credit float64
	if kind == "traffic" && h.federationOn() {
		ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
		defer cancel()
		credit, err = h.fed.ReportDonation(ctx, federationUserID(admin),
			transfer.AmountMicro, transfer.At.Unix(), signature)
		if err != nil {
			// Занос записан, и терять его из-за недоступной башки нельзя.
			// Доверие догонит следующей сверкой
			credit = 0
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"amount_micro": transfer.AmountMicro,
		"kind":         kind,
		"trust_credit": credit,
	})
}

// donationError переводит отказ цепочки в то, что можно показать человеку
func donationError(err error) string {
	switch err {
	case solana.ErrNotFound:
		return "такой транзакции в сети нет, проверьте подпись"
	case solana.ErrNotConfirmed:
		return "транзакция ещё не финализирована, попробуйте через минуту"
	case solana.ErrFailed:
		return "транзакция не прошла в сети"
	case solana.ErrNoTransfer:
		return "в этой транзакции нет перевода USDT на наш адрес"
	default:
		return "не смог проверить транзакцию: " + err.Error()
	}
}

// operatorCut спрашивает у цепочки, какую долю заноса на трафик забирает
// оператор. Не ответила - показываем ноль и молчим: раздел про деньги, а не про
// состояние RPC
func (h *Handler) operatorCut(ctx context.Context) uint16 {
	account, _ := h.store.GetPlatformSetting(settingFeeState, "")
	if strings.TrimSpace(account) == "" {
		return 0
	}
	call, cancel := context.WithTimeout(ctx, federationTimeout)
	defer cancel()
	cut, err := solana.New("").OperatorCut(call, account)
	if err != nil {
		return 0
	}
	return cut
}
