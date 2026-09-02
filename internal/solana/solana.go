// Package solana проверяет, что занос действительно случился.
//
// Верить на слово тут нельзя вообще: доверие в оракуле греется за донат, и без
// проверки любой желающий вписал бы себе чужую подпись и получил очки даром.
// Поэтому транзакция поднимается из цепочки и разбирается сама
package solana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// MainnetRPC - публичная точка. Своей ноды у нас нет, а нагрузка тут копеечная:
// один запрос на занос
const MainnetRPC = "https://api.mainnet-beta.solana.com"

// requestTimeout - публичный RPC умеет тупить, но вечно ждать его незачем
const requestTimeout = 15 * time.Second

var (
	// ErrNotFound - такой транзакции в цепочке нет
	ErrNotFound = errors.New("solana: transaction not found")
	// ErrNotConfirmed - транзакция есть, но не финализирована
	ErrNotConfirmed = errors.New("solana: transaction is not finalized")
	// ErrFailed - транзакция провалилась, денег никто не получил
	ErrFailed = errors.New("solana: transaction failed on chain")
	// ErrNoTransfer - в транзакции нет перевода на наш адрес
	ErrNoTransfer = errors.New("solana: no transfer to the expected address")
)

// Client ходит в цепочку
type Client struct {
	endpoint string
	http     *http.Client
}

func New(endpoint string) *Client {
	if endpoint == "" {
		endpoint = MainnetRPC
	}
	return &Client{endpoint: endpoint, http: &http.Client{Timeout: requestTimeout}}
}

// Transfer - то, что нашли в транзакции
type Transfer struct {
	// AmountMicro - в миллионных долях токена. У USDT шесть знаков, так что это
	// его натуральные единицы
	AmountMicro uint64
	Mint        string
	At          time.Time
}

// rpcRequest - тело запроса JSON-RPC
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

// VerifyTransfer поднимает транзакцию и ищет в ней перевод токена на адрес.
//
// Считаем по разнице балансов токен-аккаунтов, а не по разбору инструкций: так
// перевод виден независимо от того, как его собрали - напрямую, через программу
// или пачкой вместе с другими
func (c *Client) VerifyTransfer(ctx context.Context, signature, toOwner, mint string) (Transfer, error) {
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "getTransaction",
		Params: []any{signature, map[string]any{
			"encoding":                       "jsonParsed",
			"commitment":                     "finalized",
			"maxSupportedTransactionVersion": 0,
		}},
	})
	if err != nil {
		return Transfer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Transfer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Transfer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Transfer{}, fmt.Errorf("solana: rpc answered %d", resp.StatusCode)
	}

	var parsed struct {
		Result *struct {
			BlockTime int64 `json:"blockTime"`
			Meta      *struct {
				Err               any               `json:"err"`
				PreTokenBalances  []json.RawMessage `json:"preTokenBalances"`
				PostTokenBalances []json.RawMessage `json:"postTokenBalances"`
			} `json:"meta"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Transfer{}, err
	}
	if parsed.Error != nil {
		return Transfer{}, errors.New("solana: " + parsed.Error.Message)
	}
	if parsed.Result == nil || parsed.Result.Meta == nil {
		return Transfer{}, ErrNotFound
	}
	if parsed.Result.Meta.Err != nil {
		return Transfer{}, ErrFailed
	}

	before := balancesOf(parsed.Result.Meta.PreTokenBalances, toOwner, mint)
	after := balancesOf(parsed.Result.Meta.PostTokenBalances, toOwner, mint)
	if after <= before {
		return Transfer{}, ErrNoTransfer
	}
	at := time.Unix(parsed.Result.BlockTime, 0).UTC()
	if parsed.Result.BlockTime == 0 {
		at = time.Now().UTC()
	}
	return Transfer{AmountMicro: after - before, Mint: mint, At: at}, nil
}

// tokenBalance - строка баланса токен-аккаунта в ответе RPC
type tokenBalance struct {
	Mint          string `json:"mint"`
	Owner         string `json:"owner"`
	UITokenAmount struct {
		Amount   string `json:"amount"`
		Decimals int    `json:"decimals"`
	} `json:"uiTokenAmount"`
}

// balancesOf складывает балансы владельца по нужному минту
func balancesOf(raw []json.RawMessage, owner, mint string) uint64 {
	var total uint64
	for _, item := range raw {
		var row tokenBalance
		if err := json.Unmarshal(item, &row); err != nil {
			continue
		}
		if !strings.EqualFold(row.Owner, owner) || !strings.EqualFold(row.Mint, mint) {
			continue
		}
		amount, err := strconv.ParseUint(row.UITokenAmount.Amount, 10, 64)
		if err != nil {
			continue
		}
		total += amount
	}
	return total
}
