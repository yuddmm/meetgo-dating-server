// Package interest serves the interests reference (НСИ {id, value}).
package interest

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// Interest is a reference item: id for references, value as the stable i18n key.
type Interest struct {
	ID    uuid.UUID `json:"id"`
	Value string    `json:"value"`
}

// Repository reads the interests reference table.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// List returns all interests ordered by value.
func (r *Repository) List(ctx context.Context) ([]Interest, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, value FROM interests ORDER BY value`)
	if err != nil {
		return nil, fmt.Errorf("interest: list: %w", err)
	}
	defer rows.Close()

	items := make([]Interest, 0, 64)
	for rows.Next() {
		var it Interest
		if err := rows.Scan(&it.ID, &it.Value); err != nil {
			return nil, fmt.Errorf("interest: scan: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Handler exposes the interests endpoint.
type Handler struct {
	repo *Repository
}

// NewHandler constructs a Handler.
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// Routes registers the interests routes (expected under an authenticated group).
func (h *Handler) Routes(r chi.Router) {
	r.Get("/interests", h.List)
}

// List godoc
//
//	@Summary	List interests reference
//	@Tags		interests
//	@Security	BearerAuth
//	@Produce	json
//	@Success	200	{array}	Interest
//	@Router		/interests [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.List(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, items)
}
