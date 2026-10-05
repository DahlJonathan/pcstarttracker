package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pc-tracker-server/internal/auth"
	"pc-tracker-server/internal/models"
)

func TestLockProtocolOwnershipPendingConfirmationAndOfflineRecovery(t *testing.T) {
	s, id := newTestServer(t)
	s.Auth = auth.New("test-secret", time.Hour)
	user, err := s.Auth.IssueUserToken(1)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Auth.IssueUserToken(2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO users (id,email,password_hash) VALUES (2,'other@example.test','x')`); err != nil {
		t.Fatal(err)
	}
	const deviceToken = "device-test-token"
	if _, err := s.DB.Exec(`UPDATE devices SET api_token_hash = ? WHERE id = ?`, auth.HashToken(deviceToken), id); err != nil {
		t.Fatal(err)
	}
	router := s.Router()
	request := func(method, path, token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	path := "/api/v1/devices/" + id
	request("POST", path+"/lock", "", `{"locked":true}`, http.StatusUnauthorized)
	request("POST", path+"/lock", other, `{"locked":true,"password":"parent-password"}`, http.StatusNotFound)
	request("POST", path+"/lock", user, `{"locked":true}`, http.StatusConflict)
	request("POST", path+"/lock", user, `{"locked":false,"password":"short"}`, http.StatusBadRequest)
	request("POST", path+"/lock", user, `{"locked":true,"password":"parent-password"}`, http.StatusAccepted)
	w := request("GET", path+"/control", deviceToken, "", http.StatusOK)
	var command struct {
		Revision     int64  `json:"revision"`
		Locked       bool   `json:"locked"`
		PasswordHash string `json:"password_hash"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &command); err != nil {
		t.Fatal(err)
	}
	if !command.Locked || command.Revision != 1 || !auth.CheckPassword(command.PasswordHash, "parent-password") {
		t.Fatal("invalid device command")
	}
	public := request("GET", "/api/v1/devices", user, "", http.StatusOK)
	if strings.Contains(public.Body.String(), "password_hash") || strings.Contains(public.Body.String(), command.PasswordHash) {
		t.Fatal("public device response leaked recovery hash")
	}
	var dashboard struct {
		Devices []models.Device `json:"devices"`
	}
	if err := json.Unmarshal(public.Body.Bytes(), &dashboard); err != nil {
		t.Fatal(err)
	}
	if !dashboard.Devices[0].Lock.DesiredLocked || dashboard.Devices[0].Lock.AppliedRevision == 1 {
		t.Fatal("queued lock reported applied")
	}
	request("POST", path+"/control/ack", deviceToken, `{"revision":1,"locked":false}`, http.StatusConflict)
	request("POST", path+"/control/ack", deviceToken, `{"revision":1,"locked":true}`, http.StatusNoContent)
	request("POST", path+"/control/ack", deviceToken, `{"revision":1,"locked":false,"offline_unlock":true}`, http.StatusNoContent)
	w = request("GET", path+"/control", deviceToken, "", http.StatusOK)
	if err := json.Unmarshal(w.Body.Bytes(), &command); err != nil {
		t.Fatal(err)
	}
	if command.Locked {
		t.Fatal("offline unlock did not clear desired lock")
	}
	request("POST", path+"/lock", user, `{"locked":true}`, http.StatusAccepted)
	request("POST", path+"/control/ack", deviceToken, `{"revision":1,"locked":false,"offline_unlock":true}`, http.StatusConflict)
	request("POST", path+"/control/ack", deviceToken, `{"revision":2,"locked":false,"error":"screen unavailable"}`, http.StatusNoContent)
	w = request("GET", "/api/v1/devices", user, "", http.StatusOK)
	if err := json.Unmarshal(w.Body.Bytes(), &dashboard); err != nil {
		t.Fatal(err)
	}
	lock := dashboard.Devices[0].Lock
	if lock.AppliedRevision == 2 || lock.LastError == "" {
		t.Fatal("screen failure reported success")
	}
}
