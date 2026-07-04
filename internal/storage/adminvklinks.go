package storage

import "strings"

// VK Links are the admin's shared pool of VK/OK OAuth links the relay uses to mint
// TURN credentials. They live per-admin (shared across all of that admin's nodes),
// stored newline-joined in admins.vk_links, and are embedded into the wingsv://
// links the panel issues so the app can merge them into its own settings.vkLinks.

// GetAdminVKLinks returns the admin's VK Links in order, dropping blanks.
func (s *Store) GetAdminVKLinks(adminID int64) ([]string, error) {
	row := s.queryRow(`SELECT vk_links FROM admins WHERE id = ?`, adminID)
	var joined string
	if err := row.Scan(&joined); err != nil {
		return nil, err
	}
	return splitVKLinks(joined), nil
}

// SetAdminVKLinks replaces the admin's VK Links with the given list, trimming
// blanks and de-duplicating while preserving first-seen order.
func (s *Store) SetAdminVKLinks(adminID int64, links []string) error {
	_, err := s.exec(
		`UPDATE admins SET vk_links = ?, updated_at = updated_at WHERE id = ?`,
		strings.Join(dedupVKLinks(links), "\n"), adminID,
	)
	return err
}

func splitVKLinks(joined string) []string {
	if strings.TrimSpace(joined) == "" {
		return nil
	}
	return dedupVKLinks(strings.Split(joined, "\n"))
}

func dedupVKLinks(links []string) []string {
	seen := make(map[string]struct{}, len(links))
	out := make([]string, 0, len(links))
	for _, l := range links {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	return out
}
