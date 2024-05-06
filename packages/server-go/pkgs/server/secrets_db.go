package main

import (
	"database/sql"
	"errors"
	"os"
)

func InitSecretsWithName(filepath string) (*Secrets, error) {
	println("db.go initialized")
	if _, err := os.Stat(filepath); errors.Is(err, os.ErrNotExist) {
		_, err := os.Create(filepath)
		if err != nil {
			println("Failed to create db file", err.Error())
			return nil, err
		}
	}
	newDb, err := sql.Open("sqlite3", filepath)
	if err != nil {
		return nil, err
	}
	_, err = newDb.Exec("CREATE TABLE IF NOT EXISTS secrets (id INTEGER PRIMARY KEY, name TEXT, value TEXT)")
	if err != nil {
		return nil, err
	}
	return &Secrets{db: newDb}, nil
}

const defaultSecretsDbPath = "./secrets.db"

func InitSecrets() (*Secrets, error) {
	return InitSecretsWithName(defaultSecretsDbPath)
}

type Secrets struct {
	db *sql.DB
}

func (secrets *Secrets) Insert(name, value string) error {
	_, err := secrets.db.Exec("INSERT INTO secrets (name, value) VALUES (?, ?)", name, value)
	return err
}

func (secrets *Secrets) Get(name string) (string, error) {
	var value string
	err := secrets.db.QueryRow("SELECT value FROM secrets WHERE name = ?", name).Scan(&value)
	return value, err
}

func (secrets *Secrets) GetValue(name string) (string, error) {
	var value string
	err := secrets.db.QueryRow("SELECT value FROM secrets WHERE name = ?", name).Scan(&value)
	return value, err
}

func (secrets *Secrets) Update(name, value string) error {
	_, err := secrets.db.Exec("UPDATE secrets SET value = ? WHERE name = ?", value, name)
	return err
}

func (secrets *Secrets) Delete(name string) error {
	_, err := secrets.db.Exec("DELETE FROM secrets WHERE name = ?", name)
	return err
}

func (secrets *Secrets) List() ([]string, error) {
	rows, err := secrets.db.Query("SELECT name FROM secrets")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []string{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

func (secrets *Secrets) Close() error {
	return secrets.db.Close()
}
