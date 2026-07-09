package storage

import (
	"errors"

	"gorm.io/gorm"

	"v.wingsnet.org/internal/storage/dbmodel"
)

const secondsPerDay = 86400

// ClientControl carries a managed client's traffic cap, enable/disable state and
// durable usage - the fields the client list renders and the provisioning path
// enforces. Sourced from the clients row left-joined with client_traffic.
type ClientControl struct {
	Disabled        bool
	LimitBytes      uint64
	UsedBytes       uint64
	ResetPeriodDays int64
	NextResetUnix   int64
}

// Blocked reports whether the client must be denied service: manually disabled
// or over its traffic limit. Central predicate for both the provisioning gate
// and the collector's proactive deprovision.
func (c ClientControl) Blocked() bool {
	return c.Disabled || (c.LimitBytes > 0 && c.UsedBytes >= c.LimitBytes)
}

// SetClientDisabled flips a managed client's manual cutoff, owner-gated.
func (s *Store) SetClientDisabled(clientID string, ownerAdminID int64, disabled bool) error {
	v := int64(0)
	if disabled {
		v = 1
	}
	res := s.gdb.Model(&dbmodel.Client{}).
		Where("id = ? AND owner_admin_id = ?", clientID, ownerAdminID).
		Update("disabled", v)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetClientTrafficLimit sets a managed client's traffic cap (bytes; 0 = no cap)
// and its auto-reset period (days; 0 = manual reset only), owner-gated. Ensures
// a client_traffic row and (re)arms next_reset_unix when a period is set.
// Accumulated usage is untouched - clear it with ResetClientTraffic.
func (s *Store) SetClientTrafficLimit(clientID string, ownerAdminID int64, limitBytes uint64, resetPeriodDays, nowUnix int64) error {
	if resetPeriodDays < 0 {
		resetPeriodDays = 0
	}
	return s.gdb.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&dbmodel.Client{}).
			Where("id = ? AND owner_admin_id = ?", clientID, ownerAdminID).
			Update("traffic_limit_bytes", limitBytes)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		var row dbmodel.ClientTraffic
		err := tx.Where("client_id = ?", clientID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// First time tracking this client: baseline usage at the current
			// reading so the cap counts from now, not retroactively over the
			// peer's pre-existing cumulative counters.
			rx, txb, sErr := s.sumClientPeerTraffic(clientID)
			if sErr != nil {
				return sErr
			}
			next := int64(0)
			if resetPeriodDays > 0 {
				next = nowUnix + resetPeriodDays*secondsPerDay
			}
			return tx.Create(&dbmodel.ClientTraffic{
				ClientID: clientID, LastRx: rx, LastTx: txb,
				ResetPeriodDays: resetPeriodDays, NextResetUnix: next,
			}).Error
		}
		if err != nil {
			return err
		}
		// Preserve an in-flight reset window when only the byte cap changes;
		// re-arm it when the period changes or was previously unset.
		switch {
		case resetPeriodDays == 0:
			row.NextResetUnix = 0
		case row.ResetPeriodDays != resetPeriodDays || row.NextResetUnix == 0:
			row.NextResetUnix = nowUnix + resetPeriodDays*secondsPerDay
		}
		row.ResetPeriodDays = resetPeriodDays
		return tx.Save(&row).Error
	})
}

// ResetClientTraffic zeroes a managed client's accumulated usage and rebaselines
// the per-peer counters to the current reading so accounting resumes from zero,
// re-arming the next auto reset when a period is set. Owner-gated.
func (s *Store) ResetClientTraffic(clientID string, ownerAdminID int64, nowUnix int64) error {
	var count int64
	if err := s.gdb.Model(&dbmodel.Client{}).
		Where("id = ? AND owner_admin_id = ?", clientID, ownerAdminID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	rx, tx, err := s.sumClientPeerTraffic(clientID)
	if err != nil {
		return err
	}
	return s.gdb.Transaction(func(db *gorm.DB) error {
		var row dbmodel.ClientTraffic
		err := db.Where("client_id = ?", clientID).First(&row).Error
		notFound := errors.Is(err, gorm.ErrRecordNotFound)
		if err != nil && !notFound {
			return err
		}
		row.ClientID = clientID
		row.UsedBytes = 0
		row.LastRx = rx
		row.LastTx = tx
		if row.ResetPeriodDays > 0 {
			row.NextResetUnix = nowUnix + row.ResetPeriodDays*secondsPerDay
		}
		if notFound {
			return db.Create(&row).Error
		}
		return db.Save(&row).Error
	})
}

// AccumulateClientTraffic folds every managed client's current summed peer
// counters into its durable used_bytes, adding the non-negative delta since the
// last reading (a smaller reading - relay restart or peer recreation - adds
// zero) and storing the new reading as the baseline. Call once per collector
// poll, after UpsertPeerTraffic.
func (s *Store) AccumulateClientTraffic() error {
	var rows []struct {
		ClientID string
		Rx       int64
		Tx       int64
	}
	if err := s.gdb.
		Table("client_wg_peers AS cwp").
		Joins("JOIN peer_traffic AS pt ON pt.node_id = cwp.node_id AND pt.public_key = cwp.public_key").
		Group("cwp.client_id").
		Select("cwp.client_id AS client_id, COALESCE(SUM(pt.rx_bytes),0) AS rx, COALESCE(SUM(pt.tx_bytes),0) AS tx").
		Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return s.gdb.Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			rx, txb := uint64(r.Rx), uint64(r.Tx)
			var row dbmodel.ClientTraffic
			err := tx.Where("client_id = ?", r.ClientID).First(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&dbmodel.ClientTraffic{
					ClientID: r.ClientID, LastRx: rx, LastTx: txb,
				}).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			if rx >= row.LastRx {
				row.UsedBytes += rx - row.LastRx
			}
			if txb >= row.LastTx {
				row.UsedBytes += txb - row.LastTx
			}
			row.LastRx, row.LastTx = rx, txb
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RunDueTrafficResets zeroes usage for every client whose scheduled auto reset
// is due, rebaselining to the current reading and arming the next window.
// Returns the ids that were reset. Call periodically from the collector.
func (s *Store) RunDueTrafficResets(nowUnix int64) ([]string, error) {
	var due []dbmodel.ClientTraffic
	if err := s.gdb.
		Where("reset_period_days > 0 AND next_reset_unix > 0 AND next_reset_unix <= ?", nowUnix).
		Find(&due).Error; err != nil {
		return nil, err
	}
	reset := make([]string, 0, len(due))
	for i := range due {
		row := due[i]
		rx, tx, err := s.sumClientPeerTraffic(row.ClientID)
		if err != nil {
			return reset, err
		}
		row.UsedBytes = 0
		row.LastRx = rx
		row.LastTx = tx
		row.NextResetUnix = nowUnix + row.ResetPeriodDays*secondsPerDay
		if err := s.gdb.Save(&row).Error; err != nil {
			return reset, err
		}
		reset = append(reset, row.ClientID)
	}
	return reset, nil
}

// ClientControlMap returns the traffic-limit / disabled / usage state for an
// owner's clients, keyed by id, so the list can render used/limit + a progress
// bar and the enable/disable toggle in one query.
func (s *Store) ClientControlMap(ownerAdminID int64) (map[string]ClientControl, error) {
	rows, err := s.scanClientControls(s.gdb.
		Table("clients AS c").
		Joins("LEFT JOIN client_traffic AS ct ON ct.client_id = c.id").
		Where("c.owner_admin_id = ?", ownerAdminID))
	if err != nil {
		return nil, err
	}
	out := make(map[string]ClientControl, len(rows))
	for _, r := range rows {
		out[r.ID] = r.control()
	}
	return out, nil
}

// GetClientControl returns one client's control+usage state, not owner-gated -
// the provisioning path enforces by client id alone. ErrNotFound when absent.
func (s *Store) GetClientControl(clientID string) (ClientControl, error) {
	rows, err := s.scanClientControls(s.gdb.
		Table("clients AS c").
		Joins("LEFT JOIN client_traffic AS ct ON ct.client_id = c.id").
		Where("c.id = ?", clientID))
	if err != nil {
		return ClientControl{}, err
	}
	if len(rows) == 0 {
		return ClientControl{}, ErrNotFound
	}
	return rows[0].control(), nil
}

type clientControlRow struct {
	ID              string
	Disabled        int64
	TrafficLimit    uint64
	UsedBytes       uint64
	ResetPeriodDays int64
	NextResetUnix   int64
}

func (r clientControlRow) control() ClientControl {
	return ClientControl{
		Disabled:        r.Disabled != 0,
		LimitBytes:      r.TrafficLimit,
		UsedBytes:       r.UsedBytes,
		ResetPeriodDays: r.ResetPeriodDays,
		NextResetUnix:   r.NextResetUnix,
	}
}

func (s *Store) scanClientControls(q *gorm.DB) ([]clientControlRow, error) {
	var rows []clientControlRow
	err := q.Select(
		"c.id AS id, c.disabled AS disabled, c.traffic_limit_bytes AS traffic_limit, " +
			"COALESCE(ct.used_bytes,0) AS used_bytes, COALESCE(ct.reset_period_days,0) AS reset_period_days, " +
			"COALESCE(ct.next_reset_unix,0) AS next_reset_unix").
		Scan(&rows).Error
	return rows, err
}

// sumClientPeerTraffic returns the client's current summed cumulative peer rx/tx
// across all its nodes. Zero when it holds no peers.
func (s *Store) sumClientPeerTraffic(clientID string) (uint64, uint64, error) {
	var r struct {
		Rx int64
		Tx int64
	}
	err := s.gdb.
		Table("client_wg_peers AS cwp").
		Joins("JOIN peer_traffic AS pt ON pt.node_id = cwp.node_id AND pt.public_key = cwp.public_key").
		Where("cwp.client_id = ?", clientID).
		Select("COALESCE(SUM(pt.rx_bytes),0) AS rx, COALESCE(SUM(pt.tx_bytes),0) AS tx").
		Scan(&r).Error
	return uint64(r.Rx), uint64(r.Tx), err
}
