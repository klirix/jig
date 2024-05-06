package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
)

func TestSecrets(t *testing.T) {
	// Create a new server
	db, _ := InitSecretsWithName("./testing.db")
	defer db.Close()
	defer os.Remove("./testing.db")

	cli, _ := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())

	router := mainRouter(cli, db)
	t.Run("TestSecretInspect", func(t *testing.T) {

		db.Insert("test", "testval")
		defer db.Delete("test")
		// Start the server
		req := httptest.NewRequest(http.MethodGet, "/secrets/test", nil)
		key, _ := MakeKey()
		req.Header.Add("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
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
	})

	t.Run("TestSecretCreate", func(t *testing.T) {

		testval := uuid.New().String()
		name := "test-" + uuid.New().String()
		defer db.Delete(name)

		// Start the server
		req := httptest.NewRequest(http.MethodPost, "/secrets", nil)
		req.Body = io.NopCloser(strings.NewReader(`{"name":"` + name + `", "value":"` + testval + `"}`))
		key, _ := MakeKey()
		req.Header.Add("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		// Get a secret
		res := w.Result()
		defer res.Body.Close()
		if res.StatusCode != http.StatusCreated {
			t.Errorf("Expected status code 201, got %d", res.StatusCode)
			body, _ := io.ReadAll(res.Body)
			t.Errorf("error %s", string(body))
			t.FailNow()
		}

		value, err := db.GetValue(name)

		if err != nil {
			t.Errorf("Error getting value: %s", err)
		}

		if value != testval {
			t.Errorf("Expected value to be empty, got %s", value)
		}
	})

	t.Run("TestSecretDelete", func(t *testing.T) {

		name := "test-" + uuid.New().String()
		db.Insert(name, "testval")

		// Start the server
		req := httptest.NewRequest(http.MethodDelete, "/secrets/"+name, nil)
		key, _ := MakeKey()
		req.Header.Add("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Check if secret is deleted
		if w.Code != http.StatusNoContent {
			t.Errorf("Expected status code 204, got %d", w.Code)
			t.FailNow()
		}

		_, err := db.Get(name)
		if err == nil {
			t.Errorf("Expected secret to be deleted, but it still exists")
		}
	})

	t.Run("TestSecretsList", func(t *testing.T) {

		// Insert some secrets
		db.Insert("secret1", "value1")
		db.Insert("secret2", "value2")
		db.Insert("secret3", "value3")

		defer db.Delete("secret1")
		defer db.Delete("secret2")
		defer db.Delete("secret3")

		// Start the server
		req := httptest.NewRequest(http.MethodGet, "/secrets", nil)
		key, _ := MakeKey()
		req.Header.Add("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		// Code to measure
		router.ServeHTTP(w, req)
		// Check the response status code
		if w.Code != http.StatusOK {
			t.Errorf("Expected status code 200, got %d", w.Code)
			t.FailNow()
		}

		// Parse the response body
		var result types.SecretList
		body, _ := io.ReadAll(w.Body)
		err := json.Unmarshal(body, &result)
		if err != nil {
			t.Logf("body: %s", body)
			t.Errorf("Failed to decode response body: %v", err)
			t.FailNow()
		}

		// Check the number of secrets
		expectedSecrets := []string{"secret1", "secret2", "secret3"}
		if len(result.Secrets) != len(expectedSecrets) {
			t.Errorf("Expected %d secrets, got %d", len(expectedSecrets), len(result.Secrets))
			t.FailNow()
		}

		// Check the content of secrets
		for i, secret := range result.Secrets {
			if secret != expectedSecrets[i] {
				t.Errorf("Expected secret %s, got %s", expectedSecrets[i], secret)
			}
		}
	})
}
