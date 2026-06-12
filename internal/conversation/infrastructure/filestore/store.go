package filestore

import "github.com/kubewise/kubewise/internal/conversation/domain"

// Store persists conversations for the Conversation bounded context.
type Store interface {
	Save(conv *domain.Conversation) error
	LoadRecent(n int) ([]*domain.Conversation, error)
	Delete(id string) error
}
