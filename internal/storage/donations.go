package storage

import (
	"time"

	"gorm.io/gorm/clause"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// Donation - занос, как его видит панель
type Donation struct {
	Kind        string
	Reference   string
	AmountMicro int64
	At          time.Time
}

// RecordDonation записывает занос. Второе значение false, если такой уже был:
// одну транзакцию нельзя засчитать дважды, иначе доверие покупается за чужие
// деньги
func (s *Store) RecordDonation(adminID int64, kind, reference string, amountMicro int64, at time.Time) (bool, error) {
	row := dbmodel.Donation{
		AdminID: adminID, Kind: kind, Reference: reference,
		AmountMicro: amountMicro, AtUnix: at.UTC().Unix(),
	}
	res := s.gdb.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// DonationsOf - что человек занёс, свежее первым
func (s *Store) DonationsOf(adminID int64) ([]Donation, error) {
	var rows []dbmodel.Donation
	err := s.gdb.Where("admin_id = ?", adminID).Order("at DESC").Limit(100).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Donation, 0, len(rows))
	for _, row := range rows {
		out = append(out, Donation{
			Kind: row.Kind, Reference: row.Reference,
			AmountMicro: row.AmountMicro, At: time.Unix(row.AtUnix, 0).UTC(),
		})
	}
	return out, nil
}
