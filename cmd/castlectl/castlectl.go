package main

import (
	"fmt"

	"github.com/quantum/castle/pkg/castlectl/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Printf("castlectl error: %+v\n", err)
	}
}
