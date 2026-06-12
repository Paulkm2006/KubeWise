package application

import (
	"errors"
	"time"

	"github.com/kubewise/kubewise/internal/conversation/domain"
	"github.com/kubewise/kubewise/internal/conversation/infrastructure/filestore"
)

var ErrSessionNotFound = errors.New("session not found")

type SessionSummary struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
}

type SessionDetail struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Messages  []SessionMessage `json:"messages"`
}

type SessionMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type SessionService struct {
	store filestore.Store
}

func NewSessionService(st filestore.Store) *SessionService {
	return &SessionService{store: st}
}

func (s *SessionService) ListRecent(limit int) ([]SessionSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	sessions, err := s.store.LoadRecent(limit)
	if err != nil {
		return nil, err
	}
	out := make([]SessionSummary, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, toSummary(sess))
	}
	return out, nil
}

func (s *SessionService) Create(title string) (*SessionSummary, error) {
	conv := domain.NewConversation()
	if title != "" {
		conv.Title = title
	}
	if err := s.store.Save(conv); err != nil {
		return nil, err
	}
	summary := toSummary(conv)
	return &summary, nil
}

func (s *SessionService) Get(id string) (*SessionDetail, error) {
	sessions, err := s.store.LoadRecent(200)
	if err != nil {
		return nil, err
	}
	for _, sess := range sessions {
		if sess.ID == id {
			return toDetail(sess), nil
		}
	}
	return nil, ErrSessionNotFound
}

func (s *SessionService) Delete(id string) error {
	return s.store.Delete(id)
}

func toSummary(sess *domain.Conversation) SessionSummary {
	return SessionSummary{
		ID:           sess.ID,
		Title:        sess.Title,
		CreatedAt:    sess.CreatedAt,
		UpdatedAt:    sess.UpdatedAt,
		MessageCount: len(sess.Messages),
	}
}

func toDetail(sess *domain.Conversation) *SessionDetail {
	msgs := make([]SessionMessage, 0, len(sess.Messages))
	for _, m := range sess.Messages {
		msgs = append(msgs, SessionMessage{
			Role: m.Role, Content: m.Content, Timestamp: m.Timestamp,
		})
	}
	return &SessionDetail{
		ID: sess.ID, Title: sess.Title,
		CreatedAt: sess.CreatedAt, UpdatedAt: sess.UpdatedAt,
		Messages: msgs,
	}
}
