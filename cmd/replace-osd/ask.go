package main

import (
	"errors"
	"fmt"
)

func (util *UtilityImpl) AskUser(message string) error {
	fmt.Println(message)
	var userInput string
	fmt.Scan(&userInput)
	if userInput != "y" {
		return errors.New("the operation was canceled by user")
	}
	return nil
}
