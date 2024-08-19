package main

import (
	"database/sql"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

type tokenStorage struct {
	db *sql.DB
}

func InitTokenStorage(db *sql.DB) (*tokenStorage, error) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS tokens (id integer primary key, token TEXT, name TEXT, created_at TEXT, permissions INTEGER, valid_until TEXT)")
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS uniqtoken ON tokens (token);")
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS uniqname ON tokens (name);")
	if err != nil {
		return nil, err
	}
	return &tokenStorage{db: db}, nil
}

type Token struct {
	Token       string
	Name        string
	CreatedAt   time.Time
	Permissions int
	ValidUntil  time.Time
}

const (
	PermissionsRead = 1 << iota
	PermissionsDeploy
	PermissionsDelete
)

func (t *Token) CanRead() bool {
	return t.Permissions&PermissionsRead == PermissionsRead
}

func (t *Token) CanDeploy() bool {
	return t.Permissions&PermissionsDeploy == PermissionsDeploy
}

func (t *Token) CanDelete() bool {
	return t.Permissions&PermissionsDelete == PermissionsDelete
}

func (t *Token) insert(db *sql.DB) error {
	_, err := db.Exec(
		"INSERT INTO tokens (token, name, created_at, permissions) VALUES (?, ?, ?, ?)",
		t.Token, t.Name, t.CreatedAt.Format(time.RFC3339), t.Permissions,
	)
	return err
}

func (t *tokenStorage) Make(name string) (*Token, error) {
	randomSting := strings.ReplaceAll(uuid.New().String(), "-", "")
	now := time.Now()
	token := &Token{Token: randomSting, Name: name, CreatedAt: now, Permissions: PermissionsRead | PermissionsDeploy | PermissionsDelete}
	err := token.insert(t.db)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (t *tokenStorage) List() ([]Token, error) {
	rows, err := t.db.Query("SELECT token, name, created_at, permissions FROM tokens")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []Token

	for rows.Next() {
		var token string
		var name string
		var createdAt string
		var permissions int
		err = rows.Scan(&token, &name, &createdAt, &permissions)
		if err != nil {
			return nil, err
		}
		createdAtTime, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			log.Fatalf("Failed to parse created_at for token %s, check format: %s", name, err)
			return nil, err
		}

		tokens = append(tokens, Token{Token: token, Name: name, CreatedAt: createdAtTime, Permissions: permissions})
	}
	return tokens, nil
}

func (t *tokenStorage) Delete(name string) error {
	_, err := t.db.Exec("DELETE FROM tokens WHERE name = ?", name)
	return err
}

func (t *tokenStorage) Get(token string) (*Token, error) {
	row := t.db.QueryRow("SELECT token, name, created_at, permissions FROM tokens WHERE token = ?", token)
	return t.scanToken(row)
}

func (t *tokenStorage) scanToken(row *sql.Row) (*Token, error) {
	var tokenString string
	var name string
	var createdAt string
	var permissions int
	err := row.Scan(&tokenString, &name, &createdAt, &permissions)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Token not found
		}
		return nil, err
	}
	createdAtTime, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		log.Fatalf("Failed to parse created_at for token %s, check format: %s", name, err)
		return nil, err
	}
	return &Token{Token: tokenString, Name: name, CreatedAt: createdAtTime, Permissions: permissions}, nil
}
