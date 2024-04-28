package secret_db

import "database/sql"

var db *sql.DB

func Init() error {
	println("db.go initialized")
	newDb, err := sql.Open("sqlite3", "/var/jig/data.db")
	if err != nil {
		return err
	}
	db = newDb
	db.Exec("CREATE TABLE IF NOT EXISTS secrets (id INTEGER PRIMARY KEY, name TEXT, value TEXT)")
	return nil
}

func GetDb() *sql.DB {
	return db
}

func Insert(name, value string) error {
	_, err := db.Exec("INSERT INTO secrets (name, value) VALUES (?, ?)", name, value)
	return err
}

func Get(name string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM secrets WHERE name = ?", name).Scan(&value)
	return value, err
}

func GetValue(name string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM secrets WHERE name = ?", name).Scan(&value)
	return value, err
}

func Update(name, value string) error {
	_, err := db.Exec("UPDATE secrets SET value = ? WHERE name = ?", value, name)
	return err
}

func Delett(name string) error {
	_, err := db.Exec("DELETE FROM secrets WHERE name = ?", name)
	return err
}

func List() ([]string, error) {
	rows, err := db.Query("SELECT name FROM secrets")
	if err != nil {
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

func Close() error {
	return db.Close()
}
