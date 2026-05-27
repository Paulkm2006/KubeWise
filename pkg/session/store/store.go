// Package store provides persistence for conversations.
package store

import "github.com/kubewise/kubewise/pkg/session"

// Store persists conversations (save, load recent, delete).
type Store interface {
	Save(conv *session.Conversation) error
	LoadRecent(n int) ([]*session.Conversation, error)
	Delete(id string) error
}
