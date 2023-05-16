package main

import (
	"errors"
	"fmt"
)

const (
	safetyRate = 3.0
)

type Operator struct {
	rookObj Rook
	k8sObj  K8s
	utilObj Utility
}

func NewOperator(ns string) *Operator {
	return &Operator{
		rookObj: &RookImpl{
			Namespace: ns,
		},
		k8sObj:  &K8sImpl{},
		utilObj: &UtilityImpl{},
	}
}

func (op *Operator) ReplaceOSD(ns string, osdID int, deletePVC, keepOperatorStopped, skipCapacityCheck bool) error {
	err := op.checkPreCondition(ns, osdID, keepOperatorStopped)
	if err != nil {
		return err
	}

	osdDeployName := GetOSDDeploymentName(osdID)
	osdDeployExists, err := op.k8sObj.DeployExists(ns, osdDeployName)
	if err != nil {
		return err
	}
	fmt.Printf("osdDeployExists = %v\n", osdDeployExists)

	replacingOSDIsIn := false
	if osdDeployExists {
		err = op.rookObj.ValidateOSDDeploy(osdID, osdDeployName)
		if err != nil {
			return err
		}
		replacingOSDIsIn, err = op.rookObj.IsOSDIn(osdID)
		if err != nil {
			return err
		}
		if !replacingOSDIsIn {
			fmt.Printf("OSD.%d is out or does not exist.\n", osdID)
		}
	} else {
		err := op.utilObj.AskUser(fmt.Sprintf("Deployment %s does not exist, and the capacity check will be skipped. Are you sure? (y/n)", osdDeployName))
		if err != nil {
			return err
		}
	}

	exists, err := op.doesPodExists(ns, "app=rook-ceph-tools")
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("toolbox pod is not running")
	}

	status, err := op.checkHealthStatus()
	if err != nil {
		return err
	}
	fmt.Printf("Ceph health status = %s\n", status)

	if skipCapacityCheck || !osdDeployExists || !replacingOSDIsIn {
		fmt.Println("Skip capacity check.")
	} else {
		err = op.checkCapacity(osdID)
		if err != nil {
			return err
		}
	}

	err = op.purgeOSD(osdID, deletePVC, osdDeployExists)
	if err != nil {
		return err
	}

	if keepOperatorStopped {
		fmt.Println("Skipped operator restart.")
		return nil
	}

	err = op.recreateOSD()
	if err != nil {
		return err
	}

	return nil
}

func (op *Operator) checkPreCondition(ns string, osdID int, keepOperatorStopped bool) error {
	operatorExists, err := op.doesPodExists(ns, "app=rook-ceph-operator")
	if err != nil {
		return err
	}
	fmt.Printf("This program removes OSD %d in cluster %s.\n", osdID, ns)
	fmt.Printf("Current operator pod status: %v\n", func() string {
		if operatorExists {
			return "exist"
		}
		return "does not exist"
	}())
	fmt.Printf("Eventual operator pod status (after this program finishes): %v\n", func() string {
		if keepOperatorStopped {
			return "stopped"
		}
		return "running"
	}())
	fmt.Printf("Eventual replacing OSD pod status (after this program finishes): %v\n", func() string {
		if keepOperatorStopped {
			return "does not exist"
		}
		return "running"
	}())
	if keepOperatorStopped {
		fmt.Printf("Caution: This program stops rook-ceph-operator in namespace %s (if it is running) and do not restart it automatically.\n", ns)
	} else if !operatorExists {
		fmt.Printf("Caution: Currently the Rook operator is not running, and it will be started later by this program.\n")
	}
	err = op.utilObj.AskUser("Are you sure about all things shown above? (y/n)")
	if err != nil {
		return err
	}

	return nil
}

func (op *Operator) doesPodExists(ns, label string) (bool, error) {
	podNames, err := op.k8sObj.GetPodNames(ns, label)
	if err != nil {
		return false, err
	}
	return (len(podNames) != 0), nil
}

func (op *Operator) checkHealthStatus() (string, error) {
	status, err := op.rookObj.GetHealthStatus()
	if err != nil {
		return "", err
	}
	if (status != HealthOK) && (status != HealthWarn) {
		return "", fmt.Errorf("status should be %s or %s: %s", HealthOK, HealthWarn, status)
	}
	return status, nil
}

func (op *Operator) checkCapacity(osdID int) error {
	fmt.Println("Capacity check start")
	replacingOSD, err := op.rookObj.GetOSDInfo(osdID)
	if err != nil {
		if errors.Is(err, ErrOsdNotFoundError) {
			err := op.utilObj.AskUser(fmt.Sprintf("OSD.%d does not belong to the cluster, and the capacity check will be skipped. Are you sure to continue? (y/n)", osdID))
			if err != nil {
				return err
			}
			fmt.Println("Skip capacity check")
			return nil
		}
		return err
	}

	sameDeviceClassOSDs, err := op.rookObj.GetSameDeviceClassOSDs(osdID, replacingOSD.DeviceClass)
	if err != nil {
		return err
	}
	if len(sameDeviceClassOSDs) == 0 {
		err := op.utilObj.AskUser(fmt.Sprintf("All healthy OSDs in the device class %s will be removed, and the capacity check will be skipped. Are you sure? (y/n)", replacingOSD.DeviceClass))
		if err != nil {
			return err
		}
		fmt.Println("Skip capacity check")
		return nil
	}
	fmt.Println("OSD IDs in the same device class:")
	for _, osd := range sameDeviceClassOSDs {
		fmt.Printf("%d, ", osd.ID)
	}
	fmt.Println("")

	weightSum := getWeightSum(sameDeviceClassOSDs)

	for _, osd := range sameDeviceClassOSDs {
		anticipatedUsedRatio := (osd.UsedCapacity +
			safetyRate*replacingOSD.UsedCapacity*osd.Weight/weightSum) / osd.Capacity
		fmt.Printf("OSD ID: %d, anticipatedUsedRatio: %f\n", osd.ID, anticipatedUsedRatio)
		if osd.NearfullRatio < anticipatedUsedRatio {
			err := op.utilObj.AskUser(fmt.Sprintf("Usage rate of OSD.%d will possibly be over nearfull ratio(%v). Are you sure? (y/n)",
				osd.ID, osd.NearfullRatio))
			if err != nil {
				return err
			}
		}
	}

	fmt.Println("Capacity check OK")
	return nil
}

func (op *Operator) purgeOSD(osdID int, deletePVC, osdDeployExists bool) error {
	if osdDeployExists {
		deployName := GetOSDDeploymentName(osdID)
		err := op.rookObj.StopOperator()
		if err != nil {
			return err
		}

		err = op.rookObj.StopOSD(osdID)
		if err != nil {
			return err
		}

		err = op.rookObj.DeleteSubsidiaryResources(deployName, deletePVC)
		if err != nil {
			return err
		}
	}

	err := op.rookObj.DeleteCephInternalOSDInfo(osdID)
	if err != nil {
		return err
	}

	fmt.Println("osd purge finished.")

	return nil
}

func (op *Operator) recreateOSD() error {
	err := op.rookObj.StartOperator()
	if err != nil {
		return err
	}
	err = op.rookObj.WaitForAllOSDsToStart()
	if err != nil {
		return err
	}
	return nil
}

func getWeightSum(osds []*OSD) float64 {
	weightSum := 0.0
	for _, osd := range osds {
		weightSum += osd.Weight
	}
	return weightSum
}
