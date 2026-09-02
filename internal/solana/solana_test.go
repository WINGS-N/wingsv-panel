package solana

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// answer поднимает поддельный RPC: ходить в мейннет из тестов нельзя, а разбор
// ответа проверить надо
func answer(t *testing.T, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

const mint = "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB"
const wallet = "D7jqCxX6huU3tQJGTLSNPMx3sR4op4fbeq689rwm9A7g"

// Занос засчитывается по РАЗНИЦЕ балансов: так перевод виден, как бы его ни
// собрали
func TestTransferIsCountedAsBalanceGrowth(t *testing.T) {
	client := answer(t, `{"result":{"blockTime":1788000000,"meta":{"err":null,
		"preTokenBalances":[{"mint":"`+mint+`","owner":"`+wallet+`","uiTokenAmount":{"amount":"1000000","decimals":6}}],
		"postTokenBalances":[{"mint":"`+mint+`","owner":"`+wallet+`","uiTokenAmount":{"amount":"6000000","decimals":6}}]}}}`)

	got, err := client.VerifyTransfer(context.Background(), "sig", wallet, mint)
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountMicro != 5_000_000 {
		t.Fatalf("насчитали %d микро, а занесли 5 USDT", got.AmountMicro)
	}
	if got.At.IsZero() {
		t.Fatal("время транзакции потерялось")
	}
}

// Провалившаяся транзакция денег никому не принесла, и доверия за неё быть не
// должно
func TestFailedTransactionIsRejected(t *testing.T) {
	client := answer(t, `{"result":{"blockTime":1788000000,"meta":{"err":{"InstructionError":[0,"Custom"]},
		"preTokenBalances":[],"postTokenBalances":[]}}}`)
	if _, err := client.VerifyTransfer(context.Background(), "sig", wallet, mint); err != ErrFailed {
		t.Fatalf("провалившаяся транзакция принята: %v", err)
	}
}

// Перевод на ЧУЖОЙ адрес нам не занос: иначе любой вписал бы чужую подпись
func TestTransferToSomebodyElseIsNotOurs(t *testing.T) {
	client := answer(t, `{"result":{"blockTime":1788000000,"meta":{"err":null,
		"preTokenBalances":[{"mint":"`+mint+`","owner":"SysvarRent111111111111111111111111111111111","uiTokenAmount":{"amount":"0","decimals":6}}],
		"postTokenBalances":[{"mint":"`+mint+`","owner":"SysvarRent111111111111111111111111111111111","uiTokenAmount":{"amount":"9000000","decimals":6}}]}}}`)
	if _, err := client.VerifyTransfer(context.Background(), "sig", wallet, mint); err != ErrNoTransfer {
		t.Fatalf("чужой перевод засчитан нам: %v", err)
	}
}

// Другой токен тоже мимо: занесли шиткоин - это не USDT
func TestAnotherMintDoesNotCount(t *testing.T) {
	client := answer(t, `{"result":{"blockTime":1788000000,"meta":{"err":null,
		"preTokenBalances":[{"mint":"So11111111111111111111111111111111111111112","owner":"`+wallet+`","uiTokenAmount":{"amount":"0","decimals":9}}],
		"postTokenBalances":[{"mint":"So11111111111111111111111111111111111111112","owner":"`+wallet+`","uiTokenAmount":{"amount":"9000000","decimals":9}}]}}}`)
	if _, err := client.VerifyTransfer(context.Background(), "sig", wallet, mint); err != ErrNoTransfer {
		t.Fatalf("чужой токен засчитан как USDT: %v", err)
	}
}

// Выдуманной подписи в цепочке нет, и это самый частый случай
func TestUnknownSignature(t *testing.T) {
	client := answer(t, `{"result":null}`)
	if _, err := client.VerifyTransfer(context.Background(), "sig", wallet, mint); err != ErrNotFound {
		t.Fatalf("несуществующая транзакция принята: %v", err)
	}
}
