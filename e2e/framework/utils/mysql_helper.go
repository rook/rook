package utils

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/icrowley/fake"
	"math/rand"
	"strconv"
	"time"
)

type MySqlHelper struct {
	DB *sql.DB
}

// create a s3 client for specfied endpoint and creds
func CreateNewMySqlHelper(username string, password string, url string, dbname string) *MySqlHelper {
	dataSourceName := username + ":" + password + "@tcp(" + url + ")/" + dbname
	db, err := sql.Open("mysql", dataSourceName)
	if err != nil {
		panic(err)
	}

	return &MySqlHelper{DB: db}
}

func (h *MySqlHelper) PingSuccess() bool {
	inc := 0

	for inc < 10 {
		err := h.DB.Ping()

		if err == nil {
			return true
		}
		inc++
		time.Sleep(1 * time.Second)
	}

	return false
}

//func create sample Table
func (h *MySqlHelper) CreateTable() sql.Result {
	result, err := h.DB.Exec("CREATE TABLE LONGHAUL (id int NOT NULL AUTO_INCREMENT,number int,data varchar(32)" +
		", PRIMARY KEY (id) )")
	if err != nil {
		panic(err)
	}
	return result
}

//Insert Data
func (h *MySqlHelper) InsertRandomData() sql.Result {
	stmtIns, err := h.DB.Prepare("INSERT INTO LONGHAUL (number, data) VALUES ( ?, ? )") // ? = placeholder
	if err != nil {
		panic(err)
	}
	defer stmtIns.Close()

	result, err := stmtIns.Exec(rand.Intn(100000000), fake.CharactersN(20))
	if err != nil {
		panic(err)
	}

	return result
}

//Get row count
func (h *MySqlHelper) TableRowCount() (count int) {
	rows, err := h.DB.Query("SELECT COUNT(*) as count FROM LONGHAUL")
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		err := rows.Scan(&count)
		if err != nil {
			panic(err)
		}
	}
	return count

}

//check if table exists

func (h *MySqlHelper) TableExists() bool {
	_, err := h.DB.Query("SELECT 1 FROM LONGHAUL LIMIT 1 ")
	if err != nil {
		return false
	}
	return true
}

//Delete row
func (h *MySqlHelper) DeleteRandomRow() sql.Result {
	var id int
	rows, err := h.DB.Query("SELECT id FROM LONGHAUL ORDER BY RAND() LIMIT 1")
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		err := rows.Scan(&id)
		if err != nil {
			panic(err)
		}
	}

	result1, err := h.DB.Exec("DELETE FROM LONGHAUL WHERE id= " + strconv.Itoa(id))
	if err != nil {
		panic(err)
	}
	return result1
}
