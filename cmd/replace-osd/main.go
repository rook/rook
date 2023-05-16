package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log"
)

var (
	k8sObj  K8s
	utilObj Utility

	ErrOsdNotFoundError = errors.New("osd not found")
)

func init() {
	k8sObj = &K8sImpl{}
	utilObj = &UtilityImpl{}
}

func main() {
	ns := flag.String("ns", "", "namespace of the replaced OSD")
	osdID := flag.Int("osd", -1, "OSD ID to be replaced")
	deletePVC := flag.Bool("d", false, "delete pvc")
	keepOperatorStopped := flag.Bool("s", false, "keep the operator stopped")
	skipCapacityCheck := flag.Bool("skip-capacity-check", false, "skip capacity check")
	flag.Parse()

	if *ns == "" {
		log.Fatal("'ns' is required.'")
	}
	if *osdID < 0 {
		log.Fatal("'osd' is required.'")
	}

	op := NewOperator(*ns)
	err := op.ReplaceOSD(*ns, *osdID, *deletePVC, *keepOperatorStopped, *skipCapacityCheck)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("OSD replace finished")
}
