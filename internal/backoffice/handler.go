package backoffice

import (
	"net/http"

	"github.com/chava/raven/internal/httpapi"
)

type Handler struct {
	Service *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{Service: svc} }

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if prob := httpapi.DecodeJSON(r, 64*1024, &req); prob != nil {
		httpapi.WriteProblem(w, r, prob)
		return
	}
	u, err := h.Service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.Unauthorized("Kredensial tidak valid."))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, u)
}

type createAdminReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *Handler) CreateAdmin(w http.ResponseWriter, r *http.Request) {
	var req createAdminReq
	if prob := httpapi.DecodeJSON(r, 64*1024, &req); prob != nil {
		httpapi.WriteProblem(w, r, prob)
		return
	}
	u, err := h.Service.CreateAdmin(r.Context(), req.Email, req.Password, req.Role)
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.Internal(err))
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, u)
}
