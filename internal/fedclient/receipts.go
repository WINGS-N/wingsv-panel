package fedclient

import (
	"context"

	fedpb "v.wingsnet.org/internal/gen/fedpb"
	intakepb "v.wingsnet.org/internal/gen/intakepb"
)

// intake отдаёт клиента приёма поверх того же соединения: голова слушает оба
// сервиса одним портом и одним секретом
func (c *Client) intake() (intakepb.IntakeClient, error) {
	if _, err := c.dial(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return intakepb.NewIntakeClient(c.conn), nil
}

// RegisterKey отдаёт голове публичную половину ключа устройства. Без неё
// расписки проверить нечем и они все пойдут нахуй как неподписанные
func (c *Client) RegisterKey(ctx context.Context, subjectID string, key []byte) (bool, error) {
	client, err := c.intake()
	if err != nil {
		return false, err
	}
	got, err := client.RegisterKey(ctx, &intakepb.RegisterKeyRequest{
		SubjectId: subjectID, PublicKey: key,
	})
	if err != nil {
		return false, err
	}
	return got.GetChanged(), nil
}

// SubmitReceipts несёт подписанные расписки голове
func (c *Client) SubmitReceipts(ctx context.Context, subjectID string, receipts []*fedpb.TrafficReceipt, clientIP string) (*intakepb.SubmitReceiptsResponse, error) {
	client, err := c.intake()
	if err != nil {
		return nil, err
	}
	return client.SubmitReceipts(ctx, &intakepb.SubmitReceiptsRequest{
		SubjectId: subjectID, Receipts: receipts, ClientIp: clientIP,
	})
}
