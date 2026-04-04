// FILEPATH: /home/askhat/Projects/jig/pkgs/server/token_router_test.go

package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/go-chi/chi/v5"
)

func TestTokenRouter(t *testing.T) {
	// Create a new token router instance
	db, err := createOrOpenDb("./testing.db")
	if err != nil {
		log.Println("Failed to initialize embeded db")
		panic(err)
	}
	defer db.Close()
	defer os.Remove("./testing.db")

	tokens, err = InitTokenStorage(db)
	if err != nil {
		log.Println("Failed to initialize tokens storage")
		panic(err)
	}
	// cli, _ := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())

	tokenRouter := &TokenRouter{
		storage: tokens,
	}

	router := chi.NewRouter()
	router.Mount("/tokens", tokenRouter.Router())

	// Test listTokens endpoint
	t.Run("ListTokens", func(t *testing.T) {

		tokens.Make("token_name")
		defer tokens.Delete("token_name")
		tokens.Make("token_name2")
		defer tokens.Delete("token_name2")

		req, err := http.NewRequest("GET", "/tokens", nil)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}

		// Parse the response body
		var tokensResponse jigtypes.TokenListResponse
		err = json.NewDecoder(rr.Body).Decode(&tokensResponse)
		if err != nil {
			t.Fatal(err)
		}

		// Assert the response body
		if len(tokensResponse.TokenNames) != 2 {
			t.Errorf("expected 2 tokens, got %d", len(tokensResponse.TokenNames))
		}

		expectedTokenNames := map[string]bool{
			"token_name":  true,
			"token_name2": true,
		}
		for _, name := range tokensResponse.TokenNames {
			if !expectedTokenNames[name] {
				t.Errorf("unexpected token name %s", name)
			}
		}
	})

	// Test createToken endpoint
	t.Run("CreateToken", func(t *testing.T) {
		// Prepare the request body
		requestBody := `{"name": "token_name"}`

		req, err := http.NewRequest("POST", "/tokens", io.NopCloser(strings.NewReader(requestBody)))
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("expected status 201, got %d", rr.Code)
		}

	})

	// Test deleteToken endpoint
	t.Run("DeleteToken", func(t *testing.T) {

		req, err := http.NewRequest("DELETE", "/tokens/token_name", nil)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("expected status 204, got %d", rr.Code)
		}

		allTokens, err := tokens.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range allTokens {
			if token.Name == "token_name" {
				t.Fatalf("expected token_name to be deleted, got %+v", token)
			}
		}
	})
}
