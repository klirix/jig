package main

import (
	"database/sql"
	"strings"

	"github.com/google/uuid"
)

type tokenStorage struct {
	db *sql.DB
}

func Init(db *sql.DB) (*tokenStorage, error) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS tokens (token TEXT PRIMARY KEY, name TEXT, created_at TEXT, permissions INTEGER, valid_until TEXT)")
	if err != nil {
		return nil, err
	}
	return &tokenStorage{db: db}, nil
}

func (t *tokenStorage) insert(token string) error {
	_, err := t.db.Exec("INSERT INTO tokens (token) VALUES (?)", token)
	return err
}

func (t *tokenStorage) Make() error {
	randomSting := strings.ReplaceAll(uuid.New().String(), "-", "")
	return t.insert(randomSting)
}

func (t *tokenStorage) List() ([]string, error) {
	rows, err := t.db.Query("SELECT token FROM tokens")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		names = append(names, name)
	}
	return names, nil
}

func (t *tokenStorage) Delete(token string) error {
	_, err := t.db.Exec("DELETE FROM tokens WHERE token = ?", token)
	return err
}
