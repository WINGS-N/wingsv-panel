package admin

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"v.wingsnet.org/internal/solana"
)

// scanEvery - как часто смотрим, не прилетело ли денег. RPC бесплатный, запрос
// на круг один, так что чаще смысла нет, а реже человек сидит и гадает, дошло
// ли
const scanEvery = 45 * time.Second

// memoPrefix - с чего начинается код в заметке. Короткий и печатаемый руками:
// человек копирует его в кошелёк, а не расшифровывает
const memoPrefix = "wv-"

// MemoFor - код, который человек кладёт в memo перевода. Только он и связывает
// занос с аккаунтом: сам перевод говорит лишь "с кошелька X пришли деньги", а
// кто такой X, цепочка не знает
func MemoFor(adminID int64) string {
	return memoPrefix + strconv.FormatInt(adminID, 10)
}

// adminFromMemo разбирает код обратно в аккаунт
func adminFromMemo(memo string) (int64, bool) {
	memo = strings.TrimSpace(memo)
	// Кошельки любят дописывать своё, поэтому ищем код внутри строки, а не
	// требуем, чтобы заметка состояла ровно из него
	at := strings.Index(memo, memoPrefix)
	if at < 0 {
		return 0, false
	}
	tail := memo[at+len(memoPrefix):]
	end := 0
	for end < len(tail) && tail[end] >= '0' && tail[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	id, err := strconv.ParseInt(tail[:end], 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// StartDonationScan сам разбирает входящие переводы.
//
// Без этого человек тащил бы txid руками: занёс денег и сиди вставляй строчки,
// охуенный сервис
func (h *Handler) StartDonationScan(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(scanEvery)
		defer ticker.Stop()
		for {
			h.scanDonations(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (h *Handler) scanDonations(ctx context.Context) {
	wallets := h.donationWallets()
	client := solana.New("")
	for kind, wallet := range map[string]string{"traffic": wallets.Traffic, "dev": wallets.Dev} {
		if wallet == "" {
			continue
		}
		call, cancel := context.WithTimeout(ctx, time.Minute)
		incoming, err := client.RecentTransfers(call, wallet, wallets.Mint, 50, "")
		cancel()
		if err != nil {
			log.Printf("donation scan: %v", err)
			continue
		}
		for _, row := range incoming {
			adminID, ok := adminFromMemo(row.Memo)
			if !ok {
				// Занос без кода привязать не к кому: деньги дошли, спасибо, а вот
				// кому за них греть доверие - хуй пойми
				continue
			}
			h.creditDonation(ctx, adminID, kind, row)
		}
	}
}

// creditDonation записывает занос и уносит его в башку
func (h *Handler) creditDonation(ctx context.Context, adminID int64, kind string, row solana.Incoming) {
	fresh, err := h.store.RecordDonation(adminID, kind, row.Signature, int64(row.AmountMicro), row.At)
	if err != nil {
		log.Printf("donation scan: %v", err)
		return
	}
	if !fresh {
		return
	}
	log.Printf("donation scan: credited %d micro-USDT to admin %d (%s)", row.AmountMicro, adminID, kind)
	// Доверие греет только общий котёл: разработка это нам на хлеб, и продавать
	// за неё отношение оракула было бы наёбкой
	if kind != "traffic" || !h.federationOn() {
		return
	}
	call, cancel := context.WithTimeout(ctx, federationTimeout)
	defer cancel()
	if _, err := h.fed.ReportDonation(call, "user-"+strconv.FormatInt(adminID, 10),
		row.AmountMicro, row.At.Unix(), row.Signature); err != nil {
		log.Printf("donation scan: head did not take it: %v", err)
	}
}
