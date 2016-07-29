package castled

import (
	"fmt"

	"github.com/quantum/castle/pkg/cephd"
)

func Start() {
	cxt, _ := cephd.NewContext()
	key, _ := cxt.CreateKey()
	fmt.Println(key)
}
