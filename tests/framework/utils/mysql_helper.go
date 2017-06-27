/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"database/sql"
	"math/rand"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/icrowley/fake"
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
func (h *MySqlHelper) CloseConnection() {
	h.DB.Close()
}
func (h *MySqlHelper) PingSuccess() bool {
	inc := 0

	for inc < 30 {
		err := h.DB.Ping()

		if err == nil {
			return true
		}
		inc++
		time.Sleep(3 * time.Second)
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
