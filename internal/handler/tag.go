package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/pagination"
	"github.com/edmondlafaydavid/scoreplay/internal/service"
)

type TagHandler struct {
	svc *service.TagService
}

func NewTagHandler(svc *service.TagService) *TagHandler {
	return &TagHandler{svc: svc}
}

// CreateTag handles POST /tags.
// Accepts: {"name": "string"}
// Returns 201 with the created tag.
func (h *TagHandler) CreateTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidRequest, "invalid JSON body")
		return
	}

	tag, err := h.svc.Create(r.Context(), body.Name)
	if errors.Is(err, service.ErrTagNameEmpty) {
		writeError(w, http.StatusUnprocessableEntity, CodeValidationError, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, tag)
}

type tagPageResponse struct {
	Items      []model.Tag `json:"items"`
	NextCursor *string     `json:"next_cursor"`
}

// ListTags handles GET /tags.
// Query params: limit (default 20, max 100), cursor (opaque, from previous response).
// Returns 200 with paginated tags and a next_cursor for the following page.
func (h *TagHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := pagination.Params{Limit: pagination.ParseLimit(q.Get("limit"))}

	if raw := q.Get("cursor"); raw != "" {
		c, err := pagination.Decode(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidRequest, "invalid cursor")
			return
		}
		p.Cursor = c
	}

	tags, nextCursor, err := h.svc.List(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	resp := tagPageResponse{Items: tags}
	if nextCursor != nil {
		s := pagination.Encode(*nextCursor)
		resp.NextCursor = &s
	}

	writeJSON(w, http.StatusOK, resp)
}
