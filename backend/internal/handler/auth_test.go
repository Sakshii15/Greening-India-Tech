package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"
	"github.com/taskflow/backend/internal/handler"
	mw "github.com/taskflow/backend/internal/middleware"
	"github.com/taskflow/backend/internal/model"
)

var testDB *sql.DB
var testHandler *handler.Handler
var testRouter *chi.Mux

const testJWTSecret = "test-secret-key"

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		fmt.Println("TEST_DATABASE_URL not set, skipping integration tests")
		os.Exit(0)
	}

	var err error
	testDB, err = sql.Open("postgres", dsn)
	if err != nil {
		fmt.Printf("failed to connect to test db: %v\n", err)
		os.Exit(1)
	}
	defer testDB.Close()

	testHandler = handler.New(testDB, testJWTSecret)
	testRouter = chi.NewRouter()
	testRouter.Use(mw.JSONContentType)
	testRouter.Post("/auth/register", testHandler.Register)
	testRouter.Post("/auth/login", testHandler.Login)
	testRouter.Group(func(r chi.Router) {
		r.Use(mw.Auth(testJWTSecret))
		r.Get("/projects", testHandler.ListProjects)
		r.Post("/projects", testHandler.CreateProject)
		r.Get("/projects/{id}", testHandler.GetProject)
		r.Post("/projects/{id}/tasks", testHandler.CreateTask)
	})

	os.Exit(m.Run())
}

func TestRegister(t *testing.T) {
	body := `{"name":"Test User","email":"register-test@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp model.AuthResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Token == "" {
		t.Fatal("expected token in response")
	}
	if resp.User.Email != "register-test@example.com" {
		t.Fatalf("expected email register-test@example.com, got %s", resp.User.Email)
	}

	testDB.Exec(`DELETE FROM users WHERE email = 'register-test@example.com'`)
}

func TestRegisterValidation(t *testing.T) {
	body := `{"name":"","email":"","password":"12"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestLoginSuccess(t *testing.T) {
	regBody := `{"name":"Login Test","email":"login-test@example.com","password":"password123"}`
	regReq := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(regBody))
	regW := httptest.NewRecorder()
	testRouter.ServeHTTP(regW, regReq)

	body := `{"email":"login-test@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp model.AuthResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Token == "" {
		t.Fatal("expected token")
	}

	testDB.Exec(`DELETE FROM users WHERE email = 'login-test@example.com'`)
}

func TestLoginWrongPassword(t *testing.T) {
	regBody := `{"name":"Wrong PW","email":"wrongpw@example.com","password":"password123"}`
	regReq := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(regBody))
	regW := httptest.NewRecorder()
	testRouter.ServeHTTP(regW, regReq)

	body := `{"email":"wrongpw@example.com","password":"wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	testDB.Exec(`DELETE FROM users WHERE email = 'wrongpw@example.com'`)
}

func TestProtectedEndpointWithoutToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
