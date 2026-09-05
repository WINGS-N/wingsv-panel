// Package solana проверяет, что занос действительно случился.
//
// Верить на слово тут нельзя ни разу: за донат греется доверие в Oracle, и без
// проверки любой хитрожопый вписал бы себе чужой txid и получил очки даром.
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
	// Memo - метка, которую отправитель прицепил к переводу. Единственное, что
	// связывает занос с аккаунтом: сам по себе перевод говорит только "с
	// кошелька X пришли деньги", а кто такой X, цепочка не знает
	Memo string
}

// memoProgram - программа заметок. Их две, старая и новая, и кошельки пишут в
// обе, поэтому смотрим на любую
var memoPrograms = map[string]bool{
	"MemoSq4gqABAXKb96qnH8TysNcWxMyWCqXgDLGmfcHr": true,
	"Memo1UhkJRfHyvLMcVucJwxXeuD728EqVDDwQDxFMNo": true,
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
	defer func() { _ = resp.Body.Close() }()
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
				InnerInstructions []struct {
					Instructions []instruction `json:"instructions"`
				} `json:"innerInstructions"`
			} `json:"meta"`
			Transaction *struct {
				Message struct {
					Instructions []instruction `json:"instructions"`
				} `json:"message"`
			} `json:"transaction"`
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
	memo := ""
	if parsed.Result.Transaction != nil {
		memo = memoOf(parsed.Result.Transaction.Message.Instructions)
	}
	if memo == "" {
		for _, inner := range parsed.Result.Meta.InnerInstructions {
			if found := memoOf(inner.Instructions); found != "" {
				memo = found
				break
			}
		}
	}
	return Transfer{AmountMicro: after - before, Mint: mint, At: at, Memo: memo}, nil
}

// instruction - разобранная инструкция в ответе RPC. Заметка приезжает прямо в
// parsed строкой, разбирать её самим не надо
type instruction struct {
	ProgramID string          `json:"programId"`
	Parsed    json.RawMessage `json:"parsed"`
}

// memoOf вытаскивает заметку из набора инструкций
func memoOf(list []instruction) string {
	for _, item := range list {
		if !memoPrograms[item.ProgramID] {
			continue
		}
		var text string
		if err := json.Unmarshal(item.Parsed, &text); err == nil {
			return strings.TrimSpace(text)
		}
	}
	return ""
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

// Incoming - что прилетело на адрес
type Incoming struct {
	Signature string
	Transfer
}

// RecentTransfers перечисляет свежие входящие переводы на адрес.
//
// Сканим сами, а не ждём, пока человек припрётся с txid: RPC бесплатный, запрос
// один на круг, а человек занёс денег и больше ебаться ни с чем не должен
func (c *Client) RecentTransfers(ctx context.Context, owner, mint string, limit int, until string) ([]Incoming, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	params := map[string]any{"limit": limit, "commitment": "finalized"}
	if until != "" {
		// Дошли до уже разобранного - дальше в историю лезть незачем
		params["until"] = until
	}
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "getSignaturesForAddress",
		Params: []any{owner, params},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("solana: rpc answered %d", resp.StatusCode)
	}
	var parsed struct {
		Result []struct {
			Signature string `json:"signature"`
			Err       any    `json:"err"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, errors.New("solana: " + parsed.Error.Message)
	}

	out := make([]Incoming, 0, len(parsed.Result))
	for _, row := range parsed.Result {
		if row.Err != nil {
			continue
		}
		transfer, err := c.VerifyTransfer(ctx, row.Signature, owner, mint)
		if err != nil {
			// Чужой токен или вообще не перевод к нам - обычное дело на живом
			// адресе, и ронять из-за этого весь круг было бы дуростью нахуй
			continue
		}
		out = append(out, Incoming{Signature: row.Signature, Transfer: transfer})
	}
	return out, nil
}
