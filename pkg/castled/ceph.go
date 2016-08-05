package castled

import (
	"fmt"
	"log"
	"path"
	"time"
	/*
		"io/ioutil"
		"os"
	*/
	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/castle/pkg/kvstore"

	etcd "github.com/coreos/etcd/client"
	"golang.org/x/net/context"
)

const (
	CephKey = "/castle/ceph"
)

func Start(cfg Config) error {
	/*
			var keyring = `
		[mon.]
			key = AQBPxaJXi766KRAAfciBEfeqjzmOWiwhNPB5wQ==
			caps mon = "allow *"
		[client.admin]
			key = AQBUxaJXnpilGBAA3s6ONd17S33WHuqJMQrmBQ==
			auid = 0
			caps mds = "allow"
			caps mon = "allow *"
			caps osd = "allow *"
		`
			ioutil.WriteFile("/tmp/mon/tmp_keyring", []byte(keyring), 0644)

			var config = `
		[global]
			fsid=2f3c348b-0f62-4b2b-9a46-9dae126b3867
			run dir=/tmp/mon
			mon initial members = mon.a mon.b mon.c
		[mon.a]
			mon addr = 192.168.0.1
		[mon.b]
			mon addr = 192.168.0.2
		[mon.c]
			mon addr = 192.168.0.3
		`
			ioutil.WriteFile("/tmp/mon/tmp_config", []byte(config), 0644)

			// call mkfs
			cephd.Mon([]string{
				os.Args[0], // BUGBUG: remove this?
				"--mkfs",
				"--cluster=foo",
				"--id=mon.a",
				"--mon-data=/tmp/mon/mon.a",
				"--conf=/tmp/mon/tmp_config",
				"--keyring=/tmp/mon/tmp_keyring"})

			/*
				// run the mon
				cephd.Mon([]string{
					os.Args[0], // BUGBUG: remove this?
					"--cluster=foo",
					"--id=mon.a",
					"--mon-data=/tmp/mon/mon.a",
					"--conf=/tmp/mon/tmp_config"})
	*/

	// get an etcd client to coordinate with the rest of the cluster and load/save config
	etcdClient, err := kvstore.GetEtcdClient(cfg.EtcdURLs)
	if err != nil {
		return fmt.Errorf("failed to get etcdClient: %+v", err)
	}

	// load any existing cluster info that may have previously been created
	cluster, err := LoadClusterInfo(etcdClient)
	if err != nil {
		return fmt.Errorf("failed to load cluster info: %+v", err)
	}

	if !isClusterCreated(cluster) {
		// the cluster is not yet created, go ahead and create it now
		cluster, err := cephd.NewCluster()
		if err != nil {
			return fmt.Errorf("failed to create new cluster: %+v", err)
		}

		log.Printf("Created new cluster: %+v", cluster)
		err = SaveClusterInfo(cluster, etcdClient)
		if err != nil {
			return fmt.Errorf("failed to save new cluster info: %+v", err)
		}
	} else {
		// the cluster has already been created
		log.Printf("Cluster already exists: %+v", cluster)
	}

	if err := registerMonitor(cfg, etcdClient); err != nil {
		return fmt.Errorf("failed to register monitor: %+v", err)
	}

	// wait for monitor registration to complete for all expected initial monitors
	if err := waitForMonitorRegistration(cfg, etcdClient); err != nil {
		return fmt.Errorf("failed to wait for monitors to register: %+v", err)
	}

	return nil
}

func LoadClusterInfo(etcdClient etcd.KeysAPI) (cephd.Cluster, error) {
	resp, err := etcdClient.Get(context.Background(), path.Join(CephKey, "fsid"), nil)
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}
	fsid := resp.Node.Value

	secretsKey := path.Join(CephKey, "_secrets")

	resp, err = etcdClient.Get(context.Background(), path.Join(secretsKey, "monitor"), nil)
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}
	monSecret := resp.Node.Value

	resp, err = etcdClient.Get(context.Background(), path.Join(secretsKey, "admin"), nil)
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}
	adminSecret := resp.Node.Value

	return cephd.Cluster{
		Fsid:          fsid,
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
	}, nil
}

func handleLoadClusterInfoErr(err error) (cephd.Cluster, error) {
	if kvstore.IsEtcdKeyNotFound(err) {
		return cephd.Cluster{}, nil
	}
	return cephd.Cluster{}, err
}

func isClusterCreated(c cephd.Cluster) bool {
	return c.Fsid != "" && c.MonitorSecret != "" && c.AdminSecret != ""
}

func SaveClusterInfo(c cephd.Cluster, etcdClient etcd.KeysAPI) error {
	_, err := etcdClient.Set(context.Background(), path.Join(CephKey, "fsid"), c.Fsid, nil)
	if err != nil {
		return err
	}

	secretsKey := path.Join(CephKey, "_secrets")

	_, err = etcdClient.Set(context.Background(), path.Join(secretsKey, "monitor"), c.MonitorSecret, nil)
	if err != nil {
		return err
	}

	_, err = etcdClient.Set(context.Background(), path.Join(secretsKey, "admin"), c.AdminSecret, nil)
	if err != nil {
		return err
	}

	return nil
}

func registerMonitor(cfg Config, etcdClient etcd.KeysAPI) error {
	key := getMonitorEndpointKey(cfg.MonName)
	val := fmt.Sprintf("%s:%d", cfg.PrivateIPv4, 6790)

	_, err := etcdClient.Set(context.Background(), key, val, nil)
	if err == nil || kvstore.IsEtcdNodeExist(err) {
		log.Printf("registered monitor %s endpoint of %s", cfg.MonName, val)
	}
	return err
}

func getMonitorEndpointKey(name string) string {
	return fmt.Sprintf(path.Join(CephKey, "mons/%s/endpoint"), name)
}

func waitForMonitorRegistration(cfg Config, etcdClient etcd.KeysAPI) error {
	for i := range cfg.InitialMonitors {
		monName := cfg.InitialMonitors[i].Name
		key := getMonitorEndpointKey(monName)

		registered := false
		retryCount := 0
		retryMax := 40
		sleepTime := 5
		for {
			resp, err := etcdClient.Get(context.Background(), key, nil)
			if err == nil && resp != nil && resp.Node != nil && resp.Node.Value != "" {
				cfg.InitialMonitors[i].Endpoint = resp.Node.Value
				registered = true
			}

			if registered {
				log.Printf("monitor %s registered at %s", monName, cfg.InitialMonitors[i].Endpoint)
				break
			}

			retryCount++
			if retryCount > retryMax {
				return fmt.Errorf("exceeded max retry count waiting for monitor %s to register", monName)
			}

			<-time.After(time.Duration(sleepTime) * time.Second)
		}
	}

	return nil
}
