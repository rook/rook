package main

import (
	"fmt"

	"github.com/quantum/castle/pkg/castled/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Printf("castled error: %+v", err)
	}
}
