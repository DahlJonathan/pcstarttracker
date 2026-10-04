package api

import (
	"database/sql"
	"net/http"
	"strings"

	"pc-tracker-server/internal/auth"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
	Email string `json:"email"`
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if in.Email == "" || len(in.Password) < 8 {
		writeError(w, http.StatusBadRequest, "email required and password must be at least 8 chars")
		return
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash failed")
		return
	}

	res, err := s.DB.ExecContext(r.Context(),
		`INSERT INTO users (email, password_hash) VALUES (?, ?)`, in.Email, hash)
	if err != nil {
		// UNIQUE constraint -> email already registered.
		writeError(w, http.StatusConflict, "email already registered")
		return
	}
	uid, _ := res.LastInsertId()

	token, err := s.Auth.IssueUserToken(uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token failed")
		return
	}
	writeJSON(w, http.StatusCreated, authResponse{Token: token, Email: in.Email})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))

	var (
		uid  int64
		hash string
	)
	err := s.DB.QueryRowContext(r.Context(),
		`SELECT id, password_hash FROM users WHERE email = ?`, in.Email).Scan(&uid, &hash)
	if err == sql.ErrNoRows || (err == nil && !auth.CheckPassword(hash, in.Password)) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	token, err := s.Auth.IssueUserToken(uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token failed")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{Token: token, Email: in.Email})
}
