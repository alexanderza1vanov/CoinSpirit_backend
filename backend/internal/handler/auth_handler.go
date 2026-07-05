package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/auth"
	"github.com/example/invest-portfolio-platform/backend/internal/config"
	"github.com/example/invest-portfolio-platform/backend/internal/middleware"
	"github.com/example/invest-portfolio-platform/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

func RegisterAuthRoutes(router *http.ServeMux, pool *pgxpool.Pool, cfg config.Config) {
	users := repository.NewUserRepository(pool)

	router.HandleFunc("POST /auth/register", func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Email == "" || req.Password == "" || req.DisplayName == "" {
			writeError(w, http.StatusBadRequest, "email, password and display_name are required")
			return
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "password hashing failed")
			return
		}
		user, err := users.Create(r.Context(), req.Email, string(passwordHash), req.DisplayName)
		if err != nil {
			writeError(w, http.StatusConflict, "user already exists or invalid data")
			return
		}
		writeJSON(w, http.StatusCreated, user)
	})

	router.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		user, err := users.FindByEmail(r.Context(), req.Email)
		if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		access, err := auth.CreateToken(user.ID, user.Email, user.Role, cfg.JWTAccessSecret, 30*time.Minute)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "token generation failed")
			return
		}
		refresh, err := auth.CreateToken(user.ID, user.Email, user.Role, cfg.JWTRefreshSecret, 720*time.Hour)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "refresh token generation failed")
			return
		}
		writeJSON(w, http.StatusOK, tokenResponse{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresIn: 1800})
	})

	router.Handle("GET /auth/me", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		user, err := users.FindByID(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, user)
	})))

	// Для MVP refresh проверяет подпись refresh token и выпускает новую пару без таблицы сессий.
	router.HandleFunc("POST /auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
			writeError(w, http.StatusBadRequest, "refresh_token is required")
			return
		}
		claims, err := auth.ParseToken(req.RefreshToken, cfg.JWTRefreshSecret)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		access, err := auth.CreateToken(claims.UserID, claims.Email, claims.Role, cfg.JWTAccessSecret, 30*time.Minute)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "token generation failed")
			return
		}
		refresh, err := auth.CreateToken(claims.UserID, claims.Email, claims.Role, cfg.JWTRefreshSecret, 720*time.Hour)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "refresh token generation failed")
			return
		}
		writeJSON(w, http.StatusOK, tokenResponse{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresIn: 1800})
	})

	router.Handle("POST /auth/logout", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
	})))
}
