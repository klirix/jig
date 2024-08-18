package main

import (
	"database/sql"

	"modernc.org/sqlite"
)

type Collection struct {
	db   *sql.DB
	name string
}

func (c *Collection) Init() {
	c.db.Exec("CREATE TABLE IF NOT EXISTS " + c.name + " (name TEXT PRIMARY KEY, value TEXT)")
	c.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS uniqname ON " + c.name + " (name)")
}

type KVError struct {
	err      string
	sqlError *sqlite.Error
}

func (c *Collection) Insert(name, value string) error {
	_, err := c.db.Exec("INSERT INTO "+c.name+" (name, value) VALUES (?, ?)", name, value)

	if err != nil {
		if (err.(*sqlite.Error)).Code() == 19 {
			return ErrSecretExists
		}
	}
	return err
}

func (c *Collection) Get(name string) (string, error) {
	var value string
	err := c.db.QueryRow("SELECT value FROM "+c.name+" WHERE name = ?", name).Scan(&value)
	return value, err
}

func (c *Collection) Delete(name string) error {
	_, err := c.db.Exec("DELETE FROM "+c.name+" WHERE name = ?", name)
	return err
}

func (c *Collection) List() ([]string, error) {
	rows, err := c.db.Query("SELECT name FROM " + c.name)
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

func (c *Collection) Update(name, value string) error {
	_, err := c.db.Exec("UPDATE "+c.name+" SET value = ? WHERE name = ?", value, name)
	return err
}
