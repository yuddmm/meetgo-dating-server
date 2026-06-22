package meeting

import (
	"time"

	"github.com/google/uuid"
)

// likeProfile is the compact author card shown in the likes list (name + avatar).
type likeProfile struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	AvatarURL *string   `json:"avatarUrl"`
}

type likeResponse struct {
	ID        uuid.UUID   `json:"id"`
	CreatedAt time.Time   `json:"createdAt"`
	Profile   likeProfile `json:"profile"`
}

type acceptResponse struct {
	ChatID uuid.UUID `json:"chatId"`
}

func toLikeResponse(l likeRow) likeResponse {
	return likeResponse{
		ID:        l.ID,
		CreatedAt: l.CreatedAt.UTC(),
		Profile:   likeProfile{ID: l.ProfileID, Name: l.Name, AvatarURL: l.AvatarURL},
	}
}
