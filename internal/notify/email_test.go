package notify

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"discodrive/internal/db"
	"discodrive/internal/secret"
)

type stubSettings struct{ values map[string]string }

func (s stubSettings) GetSetting(_ context.Context, key string) (db.Setting, error) {
	if v, ok := s.values[key]; ok {
		return db.Setting{Key: key, Value: v}, nil
	}
	return db.Setting{}, pgx.ErrNoRows
}

func TestEmailUnconfiguredFails(t *testing.T) {
	c, _ := secret.New("0123456789abcdef0123456789abcdef")
	ch := &EmailChannel{q: stubSettings{values: map[string]string{}}, cipher: c}
	err := ch.Send(context.Background(), Message{To: "a@b", Subject: "s", Text: "t", HTML: "<p>t</p>"})
	if err == nil {
		t.Fatal("expected an 'SMTP not configured' error")
	}
}

func TestEmailChannelName(t *testing.T) {
	if (&EmailChannel{}).Name() != "email" {
		t.Fatal("channel name must be email")
	}
}
