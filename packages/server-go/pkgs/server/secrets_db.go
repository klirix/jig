package main

import (
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"

	"modernc.org/sqlite"
)

func InitSecretsWithName(pathToDb string) (*Secrets, error) {
	if _, err := os.Stat(pathToDb); errors.Is(err, os.ErrNotExist) {
		parts := strings.Split(filepath.ToSlash(pathToDb), "/")
		dir := strings.Join(parts[:len(parts)-1], "/")
		// file := parts[len(parts)-1]

		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			log.Println("Failed to create db directory", err.Error())
			return nil, err
		}

		_, err = os.Create(pathToDb)
		if err != nil {
			log.Println("Failed to create db file", err.Error())
			return nil, err
		}
	}
	newDb, err := sql.Open("sqlite", pathToDb)
	if err != nil {
		log.Printf("Failed to open db file: %v", err.Error())
		return nil, err
	}
	_, err = newDb.Exec("CREATE TABLE secrets (id INTEGER PRIMARY KEY, name TEXT, value TEXT)")
	if err != nil {
		sqliteError := (err.(*sqlite.Error))
		if sqliteError.Code() == 1 && strings.Contains(sqliteError.Error(), "table secrets already exists") {
			log.Println("db.go initialized")
			return &Secrets{db: newDb}, nil
		}
		log.Printf("Failed to create table: %v", err.Error())
		return nil, err
	}

	if _, err = newDb.Exec("create unique index uniqname on secrets (name);"); err != nil {
		sqliteError := (err.(*sqlite.Error))
		if sqliteError.Code() == 1 && strings.Contains(sqliteError.Error(), " already exists") {
			log.Println("db.go initialized")
			return &Secrets{db: newDb}, nil
		}
		log.Printf("Failed to create unique index: %v", err.Error())
		return nil, err
	}
	log.Println("db.go initialized")
	return &Secrets{db: newDb}, nil
}

const defaultSecretsDbPath = "./secrets.db"

func InitSecrets() (*Secrets, error) {
	return InitSecretsWithName(defaultSecretsDbPath)
}

type Secrets struct {
	db *sql.DB
}

var ErrSecretExists = errors.New("secret already exists")

func (secrets *Secrets) Insert(name, value string) error {
	_, err := secrets.db.Exec("INSERT INTO secrets (name, value) VALUES (?, ?)", name, value)

	if err != nil {
		if (err.(*sqlite.Error)).Code() == 19 {
			return ErrSecretExists
		}
	}
	return err
}

func (secrets *Secrets) Get(name string) (string, bool, error) {
	var value string
	err := secrets.db.QueryRow("SELECT value FROM secrets WHERE name = ?", name).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, err
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
	res, err := secrets.db.Exec("DELETE FROM secrets WHERE name = ?", name)
	res.RowsAffected()
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
