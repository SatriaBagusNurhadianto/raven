package storage

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/chava/raven/internal/httpapi"
	"github.com/chava/raven/internal/identity"
)

type Handler struct {
	Service *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{Service: svc} }

func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	gameID := identity.GameIDFrom(r.Context())
	playerID := identity.PlayerIDFrom(r.Context())
	if gameID == "" || playerID == "" {
		httpapi.WriteProblem(w, r, httpapi.Unauthorized("Token tidak valid."))
		return
	}
	slot := r.PathValue("slot")
	var req struct {
		Data        any    `json:"data"`
		ContentType string `json:"content_type"`
		Encoding    string `json:"encoding"`
		IfMatch     int64  `json:"if_match"`
		HasIfMatch  bool   `json:"-"`
	}
	if prob := httpapi.DecodeJSON(r, 16<<20, &req); prob != nil {
		httpapi.WriteProblem(w, r, prob)
		return
	}
	// if_match header takes precedence
	ifMatch := req.IfMatch
	hasIfMatch := req.HasIfMatch
	if hdr := r.Header.Get("If-Match"); hdr != "" {
		var v int64
		_, _ = fmt.Sscanf(hdr, "%d", &v)
		ifMatch = v
		hasIfMatch = true
	}
	dataBytes, err := json.Marshal(req.Data)
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.BadRequest("data tidak valid."))
		return
	}
	_ = ifMatch
	_ = hasIfMatch
	res, err := h.Service.Put(r.Context(), PutInput{
		GameID: gameID, PlayerID: playerID, Slot: slot,
		Data: dataBytes, ContentType: req.ContentType, Encoding: req.Encoding,
		IfMatch: ifMatch, HasIfMatch: hasIfMatch,
	})
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	gameID := identity.GameIDFrom(r.Context())
	playerID := identity.PlayerIDFrom(r.Context())
	if gameID == "" || playerID == "" {
		httpapi.WriteProblem(w, r, httpapi.Unauthorized("Token tidak valid."))
		return
	}
	slot := r.PathValue("slot")
	got, err := h.Service.Get(r.Context(), gameID, playerID, slot)
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, got)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	gameID := identity.GameIDFrom(r.Context())
	playerID := identity.PlayerIDFrom(r.Context())
	if gameID == "" || playerID == "" {
		httpapi.WriteProblem(w, r, httpapi.Unauthorized("Token tidak valid."))
		return
	}
	slots, err := h.Service.ListSlots(r.Context(), gameID, playerID)
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, slots)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	gameID := identity.GameIDFrom(r.Context())
	playerID := identity.PlayerIDFrom(r.Context())
	if gameID == "" || playerID == "" {
		httpapi.WriteProblem(w, r, httpapi.Unauthorized("Token tidak valid."))
		return
	}
	slot := r.PathValue("slot")
	if err := h.Service.Delete(r.Context(), gameID, playerID, slot); err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	gameID := identity.GameIDFrom(r.Context())
	playerID := identity.PlayerIDFrom(r.Context())
	if gameID == "" || playerID == "" {
		httpapi.WriteProblem(w, r, httpapi.Unauthorized("Token tidak valid."))
		return
	}
	slot := r.PathValue("slot")
	vs, err := h.Service.ListVersions(r.Context(), gameID, playerID, slot)
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, vs)
}

func (h *Handler) Delta(w http.ResponseWriter, r *http.Request) {
	gameID := identity.GameIDFrom(r.Context())
	playerID := identity.PlayerIDFrom(r.Context())
	if gameID == "" || playerID == "" {
		httpapi.WriteProblem(w, r, httpapi.Unauthorized("Token tidak valid."))
		return
	}
	// since param ignored for simplicity; return all
	slots, err := h.Service.ListSlots(r.Context(), gameID, playerID)
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, slots)
}
