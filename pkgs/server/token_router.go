package main

import (
	"encoding/json"
	"log"
	"net/http"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/go-chi/chi/v5"
)

type TokenRouter struct {
	storage *tokenStorage
}

func (t TokenRouter) Router() chi.Router {
	router := chi.NewRouter()

	router.Post("/", t.createToken)
	router.Get("/", t.listTokens)
	router.Delete("/{token}", t.deleteToken)

	return router
}

func (t *TokenRouter) createToken(w http.ResponseWriter, r *http.Request) {
	// Read the body from the json request body
	var body jigtypes.TokenCreateRequest
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	_, err = t.storage.Make(body.Name)
	if err != nil {
		log.Println("Failed to create token:", err)
		http.Error(w, "Failed to create token", http.StatusInternalServerError)
		return
	}
	respondWithJson(w, http.StatusCreated, "")
}

func (t *TokenRouter) listTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := t.storage.List()
	if err != nil {
		log.Println("Failed to list tokens:", err)
		http.Error(w, "Failed to list tokens", http.StatusInternalServerError)
		return
	}

	response := jigtypes.TokenListResponse{
		TokenNames: make([]string, len(tokens)),
	}
	for i, token := range tokens {
		response.TokenNames[i] = token.Name
	}

	respondWithJson(w, http.StatusOK, response)
}

func (t *TokenRouter) deleteToken(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	err := t.storage.Delete(token)
	if err != nil {
		log.Println("Failed to delete token:", err)
		http.Error(w, "Failed to delete token", http.StatusInternalServerError)
		return
	}

	respondWithJson(w, http.StatusOK, "")
}
