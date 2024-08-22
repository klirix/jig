package main

import (
	"encoding/json"
	"errors"
	"net/http"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/go-chi/chi/v5"
)

type SecretRouter struct {
	secret_db *Secrets
}

func (sr SecretRouter) Router() chi.Router {
	router := chi.NewRouter()
	router.Post("/", sr.createSecret)
	router.Get("/", sr.listSecrets)
	router.Delete("/{name}", sr.deleteSecret)
	router.Get("/{name}", sr.getSecret)
	return router
}

func (sr SecretRouter) createSecret(w http.ResponseWriter, r *http.Request) {
	secret_db := sr.secret_db

	var body jigtypes.NewSecretBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := secret_db.Insert(body.Name, body.Value); err != nil {
		switch {
		case errors.Is(err, ErrSecretExists):
			http.Error(w, err.Error(), http.StatusConflict)
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
}

func (sr SecretRouter) listSecrets(w http.ResponseWriter, r *http.Request) {
	secret_db := sr.secret_db

	secrets, err := secret_db.List()

	if err != nil {
		println("Failed to list secrets", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	secretList := jigtypes.SecretList{Secrets: secrets}

	secretsJson, err := json.Marshal(secretList)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(secretsJson)
}

func (sr SecretRouter) deleteSecret(w http.ResponseWriter, r *http.Request) {
	secret_db := sr.secret_db

	name := r.PathValue("name")

	err := secret_db.Delete(name)
	if err != nil {
		println("Failed to delete secret", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (sr SecretRouter) getSecret(w http.ResponseWriter, r *http.Request) {
	secret_db := sr.secret_db

	name := r.PathValue("name")
	secret, found, err := secret_db.Get(name)
	if err != nil {
		println("Failed to get secret", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "Secret not found", http.StatusNotFound)
	}

	var secretList jigtypes.SecretInspect = jigtypes.SecretInspect{Value: secret}

	secretJson, err := json.Marshal(secretList)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(secretJson)
}
