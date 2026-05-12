package storage

import (
	"time"
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
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO client_logs (client_id, stream, seq, ts, text) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, ln := range lines {
		seq := baseSeq + int64(i)
		ts := ln.TS.UTC().UnixMilli()
		if _, err := stmt.Exec(clientID, stream, seq, ts, ln.Text); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
		DELETE FROM client_logs WHERE client_id = ? AND stream = ?
		AND seq <= (
			SELECT COALESCE(MAX(seq), 0) - ? FROM client_logs WHERE client_id = ? AND stream = ?
		)`,
		clientID, stream, MaxLogLinesPerStream, clientID, stream); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReadClientLogs(clientID string, stream int32, sinceSeq int64, limit int) ([]LogLine, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.db.Query(`
		SELECT seq, ts, text FROM client_logs
		WHERE client_id = ? AND stream = ? AND seq > ?
		ORDER BY seq ASC LIMIT ?`,
		clientID, stream, sinceSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogLine
	for rows.Next() {
		var ln LogLine
		var ts int64
		if err := rows.Scan(&ln.Seq, &ts, &ln.Text); err != nil {
			return nil, err
		}
		ln.TS = time.UnixMilli(ts).UTC()
		out = append(out, ln)
	}
	return out, rows.Err()
}
