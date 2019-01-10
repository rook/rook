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
	"fmt"
	"math/rand"
	"strconv"
	"time"

	// import mysql driver
	_ "github.com/go-sql-driver/mysql"
	"github.com/icrowley/fake"
)

// MySQLHelper contains pointer to MySqlDB  and wrappers for basic object on mySql database
type MySQLHelper struct {
	DB *sql.DB
}

// CreateNewMySQLHelper creates a s3 client for specified endpoint and creds
func CreateNewMySQLHelper(username string, password string, url string, dbname string) *MySQLHelper {
	dataSourceName := username + ":" + password + "@tcp(" + url + ")/" + dbname
	db, err := sql.Open("mysql", dataSourceName)
	if err != nil {
		panic(err)
	}

	return &MySQLHelper{DB: db}
}

// CloseConnection function closes mysql connection
func (h *MySQLHelper) CloseConnection() {
	h.DB.Close()
}

// PingSuccess function is used check connection to a database
func (h *MySQLHelper) PingSuccess() bool {
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

// CreateTable func create sample Table
func (h *MySQLHelper) CreateTable() sql.Result {
	result, err := h.DB.Exec("CREATE TABLE LONGHAUL (id int NOT NULL AUTO_INCREMENT,number int,data1 varchar(10000),data2 varchar(10000),data3 varchar(10000),data4 varchar(10000),data5 varchar(10000)" +
		", PRIMARY KEY (id) )")
	if err != nil {
		panic(err)
	}
	return result
}

// InsertRandomData Inserts random Data into the table
func (h *MySQLHelper) InsertRandomData(dataSize int) sql.Result {
	stmtIns, err := h.DB.Prepare("INSERT INTO LONGHAUL (number, data1, data2, data3, data4, data5) VALUES ( ?, ?, ?, ?, ? )") // ? = placeholder
	if err != nil {
		panic(err)
	}
	defer stmtIns.Close()

	result, err := stmtIns.Exec(rand.Intn(100000000), fake.CharactersN(dataSize), fake.CharactersN(dataSize), fake.CharactersN(dataSize), fake.CharactersN(dataSize), fake.CharactersN(dataSize))
	if err != nil {
		panic(err)
	}

	return result
}

// TableRowCount gets row count of table
func (h *MySQLHelper) SelectRandomData(limit int) *sql.Rows {
	query := fmt.Sprintf("SELECT * FROM  LONGHAUL ORDER BY RAND() LIMIT %d", limit)
	rows, err := h.DB.Query(query)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	return rows
}

// TableRowCount gets row count of table
func (h *MySQLHelper) TableRowCount() (count int) {
	rows, err := h.DB.Query("SELECT COUNT(*) as count FROM LONGHAUL")
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(&count)
		if err != nil {
			panic(err)
		}
	}
	return count
}

// TableExists checks if a table exists
func (h *MySQLHelper) TableExists() bool {
	_, err := h.DB.Query("SELECT 1 FROM LONGHAUL LIMIT 1 ")
	if err != nil {
		return false
	}
	return true
}

// DeleteRandomRow deletes a random row
func (h *MySQLHelper) DeleteRandomRow() sql.Result {
	var id int
	rows, err := h.DB.Query("SELECT id FROM LONGHAUL ORDER BY RAND() LIMIT 1")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

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
