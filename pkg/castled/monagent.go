package castled

import "github.com/quantum/clusterd/pkg/orchestrator"

type monAgent struct {
}

func (m *monAgent) ConfigureAgent(context *orchestrator.ClusterContext, changeList []orchestrator.ChangeElement) error {
	monNames := "foo:1235"
	initMonitorNames := "mon1"
	devices := "sda,sdb"
	forceFormat := true
	privateIP4 := "1.2.3.4"
	config := NewConfig(context.EtcdClient, "default", privateIP4, monNames, initMonitorNames, devices, forceFormat)
	_, err := Bootstrap(config, context.Executor)

	return err
}

func (m *monAgent) DestroyAgent(context *orchestrator.ClusterContext) error {
	return nil
}
