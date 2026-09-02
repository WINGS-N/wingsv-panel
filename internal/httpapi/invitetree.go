package httpapi

import (
	"context"
	"log"
	"strconv"
	"time"

	"v.wingsnet.org/internal/fedclient"
	"v.wingsnet.org/internal/gen/headpb"
	"v.wingsnet.org/internal/storage"
)

// inviteTreeEvery - как часто башке обновляют карту приглашений. Дерево растёт
// медленно, а вот отозванный инвайт должен доехать до суда в разумный срок
const inviteTreeEvery = 15 * time.Minute

// startInviteTreeReports держит карту приглашений в башке свежей.
//
// Башка про людей не знает ничего и знать не должна, поэтому уезжают только
// идентификаторы: кто в чьём поддереве. Ни имён, ни почты, ни адресов
func startInviteTreeReports(ctx context.Context, store *storage.Store, fed *fedclient.Client) {
	if fed == nil || !fed.Enabled() {
		return
	}
	go func() {
		ticker := time.NewTicker(inviteTreeEvery)
		defer ticker.Stop()
		for {
			reportInviteTree(ctx, store, fed)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func reportInviteTree(ctx context.Context, store *storage.Store, fed *fedclient.Client) {
	ancestry, err := store.InviteAncestry()
	if err != nil {
		log.Printf("invite tree report: %v", err)
		return
	}
	subjects := make([]*headpb.SubjectAncestry, 0, len(ancestry))
	for child, chain := range ancestry {
		donors := make([]string, 0, len(chain))
		for _, parent := range chain {
			donors = append(donors, "admin-"+strconv.FormatInt(parent, 10))
		}
		subjects = append(subjects, &headpb.SubjectAncestry{
			SubjectId: "user-" + strconv.FormatInt(child, 10),
			DonorIds:  donors,
		})
	}
	call, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := fed.ReportInviteTree(call, subjects); err != nil {
		// Башка без суда над нодами отвечает Unimplemented, и это не поломка:
		// карта просто никому не нужна
		log.Printf("invite tree report: %v", err)
	}
}
