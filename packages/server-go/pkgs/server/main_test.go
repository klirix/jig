package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/client"
	// "github.com/askhat/jig/pkgs/server"
)

func TestSecretInspect(t *testing.T) {
	// Create a new server
	db, _ := InitSecrets()
	defer db.Close()

	cli, _ := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())

	secretsMux := MainRouter(cli, db)

	db.Insert("test", "testval")
	defer db.Delete("test")
	// Start the server
	req := httptest.NewRequest(http.MethodGet, "/secrets/test", nil)
	key, _ := MakeKey()
	req.Header.Add("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()

	secretsMux.ServeHTTP(w, req)
	// Get a secret
	res := w.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", res.StatusCode)
		t.FailNow()
	}

	var secret types.SecretInspect

	bodyRes, _ := io.ReadAll(res.Body)

	json.Unmarshal(bodyRes, &secret)

	if secret.Value != "testval" {
		t.Errorf("Expected value testval, got %s", secret.Value)
	}
}

func TestSecretCreate(t *testing.T) {
	// Create a new server
	db, _ := InitSecrets()
	defer db.Close()

	cli, _ := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())

	secretsMux := MainRouter(cli, db)

	// Start the server
	req := httptest.NewRequest(http.MethodPost, "/secrets", nil)
	key, _ := MakeKey()
	req.Header.Add("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()

	secretsMux.ServeHTTP(w, req)
	// Get a secret
	res := w.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", res.StatusCode)
		t.FailNow()
	}

	var secret types.SecretInspect

	bodyRes, _ := io.ReadAll(res.Body)

	json.Unmarshal(bodyRes, &secret)

	if secret.Value != "testval" {
		t.Errorf("Expected value testval, got %s", secret.Value)
	}
}
