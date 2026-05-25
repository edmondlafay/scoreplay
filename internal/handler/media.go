package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/pagination"
	"github.com/edmondlafaydavid/scoreplay/internal/service"
)

// multipartMemory is the max bytes held in RAM during multipart parsing.
// Parts exceeding this spill to OS temp files; the actual file-size limit
// is enforced downstream by io.LimitedReader in the service layer.
// RemoveAll() in CreateMedia cleans up any temp files after the request.
const multipartMemory = 32 << 20 // 32 MB

type MediaHandler struct {
	svc *service.MediaService
}

func NewMediaHandler(svc *service.MediaService) *MediaHandler {
	return &MediaHandler{svc: svc}
}

// CreateMedia handles POST /media.
// Accepts multipart/form-data with fields:
//   - name (string, required)
//   - tags (comma-separated tag IDs, optional)
//   - file (binary, required)
//
// Returns 201 with the created media object.
func (h *MediaHandler) CreateMedia(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(multipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidRequest, "failed to parse multipart form")
		return
	}
	defer r.MultipartForm.RemoveAll() //nolint:errcheck

	name := r.FormValue("name")

	var tagIDs []string
	if raw := r.FormValue("tags"); raw != "" {
		for _, id := range strings.Split(raw, ",") {
			if id = strings.TrimSpace(id); id != "" {
				tagIDs = append(tagIDs, id)
			}
		}
	}
	// Also support tags[] form fields
	for _, id := range r.Form["tags[]"] {
		if id = strings.TrimSpace(id); id != "" {
			tagIDs = append(tagIDs, id)
		}
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidRequest, "file is required")
		return
	}
	defer file.Close()

	media, err := h.svc.Create(r.Context(), service.CreateMediaInput{
		Name:     name,
		TagIDs:   tagIDs,
		File:     file,
		Filename: header.Filename,
	})

	switch {
	case errors.Is(err, service.ErrMediaNameEmpty):
		writeError(w, http.StatusUnprocessableEntity, CodeValidationError, err.Error())
	case errors.Is(err, service.ErrTagsNotFound):
		writeError(w, http.StatusUnprocessableEntity, CodeTagsNotFound, err.Error())
	case errors.Is(err, service.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, CodeUnsupportedFormat, err.Error())
	case errors.Is(err, service.ErrFileTooLarge):
		writeError(w, http.StatusUnprocessableEntity, CodeFileTooLarge, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
	default:
		writeJSON(w, http.StatusCreated, media)
	}
}

type mediaPageResponse struct {
	Items      []model.Media `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

// ListMedia handles GET /media.
// Query params:
//   - tag_id (repeatable) — intersect filter: only media with ALL given tags
//   - limit  (default 20, max 100)
//   - cursor (opaque, from previous response next_cursor)
func (h *MediaHandler) ListMedia(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tagIDs := q["tag_id"]
	p := pagination.Params{Limit: pagination.ParseLimit(q.Get("limit"))}

	if raw := q.Get("cursor"); raw != "" {
		c, err := pagination.Decode(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidRequest, "invalid cursor")
			return
		}
		p.Cursor = c
	}

	media, nextCursor, err := h.svc.Search(r.Context(), tagIDs, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	resp := mediaPageResponse{Items: media}
	if nextCursor != nil {
		s := pagination.Encode(*nextCursor)
		resp.NextCursor = &s
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetMedia handles GET /media/{id}.
// Returns 200 with the media object or 404 if not found.
func (h *MediaHandler) GetMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	media, err := h.svc.GetByID(r.Context(), id)
	if errors.Is(err, service.ErrMediaNotFound) {
		writeError(w, http.StatusNotFound, CodeMediaNotFound, "media not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, media)
}
