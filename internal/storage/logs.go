package storage

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"v.wingsnet.org/internal/storage/dbmodel"
)

const MaxLogLinesPerStream = 10000

type LogLine struct {
	Seq  int64
	TS   time.Time
	Text string
}

func (s *Store) AppendClientLogs(clientID string, stream int32, baseSeq int64, lines []LogLine) error {
	if len(lines) == 0 {
		return nil
	}
	rows := make([]dbmodel.ClientLog, len(lines))
	for i, ln := range lines {
		rows[i] = dbmodel.ClientLog{
			ClientID: clientID,
			Stream:   int64(stream),
			Seq:      baseSeq + int64(i),
			Ts:       ln.TS.UTC().UnixMilli(),
			Text:     ln.Text,
		}
	}
	return s.gdb.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
			return err
		}
		// Trim to the most recent MaxLogLinesPerStream. Compute the threshold in
		// two steps rather than a self-referencing subquery, which MySQL rejects.
		var maxSeq int64
		if err := tx.Model(&dbmodel.ClientLog{}).
			Where("client_id = ? AND stream = ?", clientID, stream).
			Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error; err != nil {
			return err
		}
		threshold := maxSeq - MaxLogLinesPerStream
		return tx.Where("client_id = ? AND stream = ? AND seq <= ?", clientID, stream, threshold).
			Delete(&dbmodel.ClientLog{}).Error
	})
}

func (s *Store) ReadClientLogs(clientID string, stream int32, sinceSeq int64, limit int) ([]LogLine, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	var rows []dbmodel.ClientLog
	if err := s.gdb.
		Where("client_id = ? AND stream = ? AND seq > ?", clientID, stream, sinceSeq).
		Order("seq ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	var out []LogLine
	for _, r := range rows {
		out = append(out, LogLine{Seq: r.Seq, TS: time.UnixMilli(r.Ts).UTC(), Text: r.Text})
	}
	return out, nil
}
