package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/taskflow/backend/internal/model"
	"golang.org/x/crypto/bcrypt"
)

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req model.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fields := make(map[string]string)
	if strings.TrimSpace(req.Name) == "" {
		fields["name"] = "is required"
	}
	if strings.TrimSpace(req.Email) == "" {
		fields["email"] = "is required"
	}
	if len(req.Password) < 6 {
		fields["password"] = "must be at least 6 characters"
	}
	if len(fields) > 0 {
		writeValidationError(w, fields)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		slog.Error("bcrypt error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var user model.User
	err = h.db.QueryRow(
		`INSERT INTO users (name, email, password) VALUES ($1, $2, $3)
		 RETURNING id, name, email, created_at`,
		strings.TrimSpace(req.Name), strings.ToLower(strings.TrimSpace(req.Email)), string(hash),
	).Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeValidationError(w, map[string]string{"email": "already registered"})
			return
		}
		slog.Error("insert user error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	token, err := h.generateToken(user.ID, user.Email)
	if err != nil {
		slog.Error("token generation error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, model.AuthResponse{Token: token, User: user})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fields := make(map[string]string)
	if strings.TrimSpace(req.Email) == "" {
		fields["email"] = "is required"
	}
	if req.Password == "" {
		fields["password"] = "is required"
	}
	if len(fields) > 0 {
		writeValidationError(w, fields)
		return
	}

	var user model.User
	err := h.db.QueryRow(
		`SELECT id, name, email, password, created_at FROM users WHERE email = $1`,
		strings.ToLower(strings.TrimSpace(req.Email)),
	).Scan(&user.ID, &user.Name, &user.Email, &user.Password, &user.CreatedAt)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := h.generateToken(user.ID, user.Email)
	if err != nil {
		slog.Error("token generation error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user.Password = ""
	writeJSON(w, http.StatusOK, model.AuthResponse{Token: token, User: user})
}

func (h *Handler) generateToken(userID, email string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.jwtSecret))
}
