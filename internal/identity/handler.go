package identity

import (
	"context"
	"net/http"
	"strings"

	"github.com/chava/raven/internal/httpapi"
)

// Handler exposes auth endpoints.
type Handler struct {
	Service *Service
}

// NewHandler creates a handler.
func NewHandler(svc *Service) *Handler { return &Handler{Service: svc} }

type guestReq struct {
	GameID   string `json:"game_id"`
	DeviceID string `json:"device_id"`
}

func (h *Handler) Guest(w http.ResponseWriter, r *http.Request) {
	var req guestReq
	if prob := httpapi.DecodeJSON(r, 64*1024, &req); prob != nil {
		httpapi.WriteProblem(w, r, prob)
		return
	}
	if req.GameID == "" {
		httpapi.WriteProblem(w, r, httpapi.BadRequest("game_id wajib diisi."))
		return
	}
	tp, err := h.Service.Guest(r.Context(), req.GameID, req.DeviceID, r.UserAgent(), httpapi.ClientIPFrom(r.Context()))
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, tp)
}

type registerReq struct {
	GameID   string `json:"game_id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if prob := httpapi.DecodeJSON(r, 64*1024, &req); prob != nil {
		httpapi.WriteProblem(w, r, prob)
		return
	}
	if req.GameID == "" {
		httpapi.WriteProblem(w, r, httpapi.BadRequest("game_id wajib diisi."))
		return
	}
	tp, err := h.Service.Register(r.Context(), req.GameID, req.Email, req.Password, r.UserAgent(), httpapi.ClientIPFrom(r.Context()))
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, tp)
}

type loginReq struct {
	GameID   string `json:"game_id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if prob := httpapi.DecodeJSON(r, 64*1024, &req); prob != nil {
		httpapi.WriteProblem(w, r, prob)
		return
	}
	if req.GameID == "" {
		httpapi.WriteProblem(w, r, httpapi.BadRequest("game_id wajib diisi."))
		return
	}
	tp, err := h.Service.Login(r.Context(), req.GameID, req.Email, req.Password, r.UserAgent(), httpapi.ClientIPFrom(r.Context()))
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, tp)
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshReq
	if prob := httpapi.DecodeJSON(r, 64*1024, &req); prob != nil {
		httpapi.WriteProblem(w, r, prob)
		return
	}
	tp, err := h.Service.Refresh(r.Context(), req.RefreshToken, r.UserAgent(), httpapi.ClientIPFrom(r.Context()))
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, tp)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req refreshReq
	if prob := httpapi.DecodeJSON(r, 64*1024, &req); prob != nil {
		// body kosong diperbolehkan; coba baca dari query
	}
	_ = h.Service.Logout(r.Context(), req.RefreshToken)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	pid := PlayerIDFrom(r.Context())
	if pid == "" {
		httpapi.WriteProblem(w, r, httpapi.Unauthorized("Token tidak valid."))
		return
	}
	if err := h.Service.LogoutAll(r.Context(), pid); err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type linkReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Link(w http.ResponseWriter, r *http.Request) {
	pid := PlayerIDFrom(r.Context())
	gid := GameIDFrom(r.Context())
	if pid == "" || gid == "" {
		httpapi.WriteProblem(w, r, httpapi.Unauthorized("Token tidak valid."))
		return
	}
	var req linkReq
	if prob := httpapi.DecodeJSON(r, 64*1024, &req); prob != nil {
		httpapi.WriteProblem(w, r, prob)
		return
	}
	if err := h.Service.Link(r.Context(), pid, gid, req.Email, req.Password); err != nil {
		httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AuthMiddleware validates access token and injects player/game into context.
type AuthMiddleware struct {
	JWT     *JWTIssuer
	Service *Service
	Cache   *banCache
}

func NewAuthMiddleware(jwt *JWTIssuer, svc *Service) *AuthMiddleware {
	return &AuthMiddleware{JWT: jwt, Service: svc, Cache: newBanCache()}
}

func (m *AuthMiddleware) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			httpapi.WriteProblem(w, r, httpapi.Unauthorized("Bearer token wajib diisi."))
			return
		}
		token := strings.TrimSpace(auth[len("Bearer "):])
		claims, err := m.JWT.Verify(token)
		if err != nil {
			httpapi.WriteProblem(w, r, httpapi.Unauthorized("Access token tidak valid atau kadaluarsa."))
			return
		}
		// Ban check dengan cache TTL 30s (FR-A6 <=60s).
		banned, err := m.Cache.check(r.Context(), claims.PlayerID, m.Service.IsBanned)
		if err != nil {
			httpapi.WriteProblem(w, r, httpapi.Internal(err))
			return
		}
		if banned {
			httpapi.WriteProblem(w, r, httpapi.Banned("Pemain diblokir."))
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, CtxPlayerID, claims.PlayerID)
		ctx = context.WithValue(ctx, CtxGameID, claims.GameID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
