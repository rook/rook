package clusterd

import (
	"errors"
	"io/ioutil"
	"log"
	"path"
	"strings"
	"sync/atomic"
	"time"

	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
	ctx "golang.org/x/net/context"
)

const (
	HeartbeatKey                     = "heartbeat"
	SetAction                        = "set"
	CreateAction                     = "create"
	LeaseVersion                     = 1
	LeaseTtlSeconds                  = 15
	LeaseRetrySeconds                = 5
	HeartbeatTtlSeconds              = 60 * 60
	HardwareDiscoveryIntervalSeconds = 120 * 60
	WatchErrorRetrySeconds           = 2
	MachineTtlMinutes                = 5
	maxMachineIDLength               = 12
)

var (
	LeaseTtlDuration          = time.Duration(LeaseTtlSeconds) * time.Second
	LeaseRetryDuration        = time.Duration(LeaseRetrySeconds) * time.Second
	MachineTtlDuration        = time.Duration(MachineTtlMinutes) * time.Minute
	HeartbeatTtlDuration      = time.Duration(HeartbeatTtlSeconds) * time.Second
	HardwareDiscoveryInterval = time.Duration(HardwareDiscoveryIntervalSeconds) * time.Second
)

// Interface defining how a leader reacts to gaining or losing election, new members being added, etc.
type Leader interface {
	OnLeadershipAcquired() error
	OnLeadershipLost() error
	GetLeaseName() string
}

func GetMachineID() (string, error) {
	buf, err := ioutil.ReadFile("/etc/machine-id")
	if err != nil {
		return "", err
	}

	return trimMachineID(string(buf)), nil
}

func trimMachineID(id string) string {
	// Trim the machine ID to a length that is statistically unlikely to collide with another node in the cluster
	// while allowing us to use an ID that is both unique and succinct.
	// Using the birthday collision algorithm, if we have a length of 12 hex characters, that gives us
	// 16^12 possibilities. If we have a cluster with 1,000 nodes, we have a likelihood with node IDs
	// colliding in less than 1 in a billion clusters.
	id = strings.TrimSpace(id)
	if len(id) <= maxMachineIDLength {
		return id
	}

	return id[0:maxMachineIDLength]
}

func IsLeader(l Lease, nodeID string) bool {
	if l == nil {
		return false
	}

	if l.MachineID() != nodeID {
		return false
	}

	return true
}

type ClusterMember struct {
	context               *Context
	isLeader              bool
	leaseManager          Manager
	leader                Leader
	hardwareDiscoveryLock int32
}

func newClusterMember(context *Context, leaseManager Manager, leader Leader) *ClusterMember {
	return &ClusterMember{
		context:               context,
		isLeader:              false,
		leaseManager:          leaseManager,
		leader:                leader,
		hardwareDiscoveryLock: 0,
	}
}

func (r *ClusterMember) initialize() error {
	err := r.ElectLeader()
	if err != nil {
		return err
	}

	// in a goroutine, begin the monitor cluster loop for changes in membership, leadership, etc.
	go func() {
		r.RefreshLeader()
	}()

	go func() {
		r.DiscoverHardwareLoop()
	}()

	go func() {
		r.WaitForHardwareChangeNotifications()
	}()

	return nil
}

func (r *ClusterMember) ElectLeader() error {
	// keep our cluster membership up to date
	err := r.heartbeat()
	if err != nil {
		log.Printf("failed to heartbeat, will try again later: err=%v", err)
	}

	existing, err := r.leaseManager.GetLease(r.leader.GetLeaseName())
	if err != nil {
		// failed to get the current lease, ensure our leadership status is updated
		r.updateLeaderStatus(err)
		return err
	}

	var l Lease
	if existing == nil {
		// no leader currently exists, try to acquire
		l, err = r.leaseManager.AcquireLease(r.leader.GetLeaseName(), r.context.NodeID, LeaseVersion, LeaseTtlDuration)
		if err != nil {
			// failed to acquire lease, ensure our leadership status is updated
			r.updateLeaderStatus(err)
			return err
		} else if l == nil {
			// failed to acquire lease. This node is simply not the leader
			r.updateLeaderStatus(errors.New("another node is leader"))
			return nil
		}

		// succeeded in acquiring lease, ensure our leadership status is updated
		r.updateLeaderStatus(nil)
	} else if IsLeader(existing, r.context.NodeID) {
		// we are the existing leader, attempt to renew the lease then ensure our leadership status is updated
		err = existing.Renew(LeaseTtlDuration)
		r.updateLeaderStatus(err)
		return err
	} else if r.isLeader {
		// we used to be the leader, now we are not
		r.isLeader = false
		return r.leader.OnLeadershipLost()
	}

	return nil
}

func (r *ClusterMember) RefreshLeader() {
	for {
		// sleep for a portion of the lease TTL and try again
		<-time.After(LeaseRetryDuration)

		err := r.ElectLeader()
		if err != nil {
			log.Printf("error while electing leader: %s", err.Error())
		}
	}
}

func (r *ClusterMember) DiscoverHardwareLoop() {
	for {
		err := r.discoverHardware()
		if err != nil {
			log.Printf("error while discovering hardware: %+v", err)
		} else {
			log.Print("hardware discovery complete")
		}

		// sleep until it's time to detect hardware again
		<-time.After(HardwareDiscoveryInterval)
	}
}

func (r *ClusterMember) WaitForHardwareChangeNotifications() {
	hardwareTriggerKey := path.Join(inventory.DiscoveredNodesKey, r.context.NodeID, inventory.TriggerHardwareDetectionKey)
	hardwareWatcher := r.context.EtcdClient.Watcher(hardwareTriggerKey, nil)
	for {
		// wait for any changes to the hardware detection trigger key
		resp, err := hardwareWatcher.Next(ctx.Background())
		if err != nil {
			if err == ctx.Canceled {
				log.Print("hardware change watching cancelled, bailing out...")
				break
			} else {
				<-time.After(time.Duration(WatchErrorRetrySeconds) * time.Second)
				continue
			}
		}

		if resp != nil && resp.Node != nil && resp.Action == SetAction {
			// the trigger hardware detection key was set, perform a hardware discovery
			err := r.discoverHardware()
			if err != nil {
				log.Printf("error while discovering hardware after a change notification: %+v", err)
			} else {
				log.Print("hardware discovery after a change notification complete")
			}

			// clear out the hardware detection trigger key now so it can fire again later
			r.context.EtcdClient.Delete(ctx.Background(), hardwareTriggerKey, nil)
		}
	}
}

func (r *ClusterMember) updateLeaderStatus(err error) error {
	if err != nil && r.isLeader {
		// there was an error in leader election and we currently think we're the leader
		// update our internal state
		r.onLeadershipLost()
	} else if err == nil && !r.isLeader {
		// there was no error in leader election and we currently don't think we're the leader
		// update our internal state
		return r.onLeadershipAcquired()
	}

	// Return the error that indicates the original cause for not updating the status
	return err
}

func (r *ClusterMember) onLeadershipAcquired() error {
	r.isLeader = true
	log.Printf("cluster leadership acquired by this machine (%s)", r.context.NodeID)

	return r.leader.OnLeadershipAcquired()
}

func (r *ClusterMember) onLeadershipLost() {
	log.Print("leadership lost by this machine")
	r.isLeader = false
	r.leader.OnLeadershipLost()
}

func (r *ClusterMember) heartbeat() error {
	machineKey := path.Join(inventory.DiscoveredNodesHealthKey, r.context.NodeID)

	// first ensure the machine key (directory) exists
	_, err := r.context.EtcdClient.Set(ctx.Background(), machineKey, "", &etcd.SetOptions{Dir: true})
	if err != nil && !util.IsEtcdDirAlreadyExists(err) {
		// we got a different error than "it already exists", bail out
		return err
	}

	// send a heartbeat by setting the heartbeat etcd key.  we are avoiding setting a timestamp
	// here since cluster member clock may be skewed from etcd cluster clocks.  note that last
	// heartbeat time can be indirectly determined by getting the key and subtracting the current
	// TTL from the original TTL
	heartbeatKey := path.Join(machineKey, HeartbeatKey)
	_, err = r.context.EtcdClient.Set(ctx.Background(), heartbeatKey, "", &etcd.SetOptions{TTL: HeartbeatTtlDuration})
	return err
}

func (r *ClusterMember) discoverHardware() error {
	hardwareDiscoveryCount := atomic.AddInt32(&r.hardwareDiscoveryLock, 1)
	defer atomic.AddInt32(&r.hardwareDiscoveryLock, -1)

	if hardwareDiscoveryCount != 1 {
		log.Print("local hardware discovery already in progress, skipping...")
		return nil
	}

	// Discover current state of the disks and other node properties

	return nil
}
