package main

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "./data/lexicon.db")
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, path, mime, media_kind FROM tracks WHERE id IN (439, 443, 448)")
	if err != nil {
		fmt.Println("query err:", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var path, mime, kind string
		rows.Scan(&id, &path, &mime, &kind)
		fmt.Printf("id=%d path=%q mime=%q kind=%q\n", id, path, mime, kind)
	}

	// Also check a few regular tracks for comparison
	fmt.Println("---regular tracks---")
	rows2, _ := db.Query("SELECT id, path, mime FROM tracks WHERE id < 5")
	defer rows2.Close()
	for rows2.Next() {
		var id int64
		var path, mime string
		rows2.Scan(&id, &path, &mime)
		fmt.Printf("id=%d path=%q mime=%q\n", id, path, mime)
	}
}
