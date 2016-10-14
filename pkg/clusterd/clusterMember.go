package clusterd

import (
	"errors"
	"fmt"
	"log"
	"path"
	"sync/atomic"
	"time"

	etcd "github.com/coreos/etcd/client"
	"github.com/coreos/etcd/store"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
	ctx "golang.org/x/net/context"
)

const (
	SystemLogDir                     = "/var/log/castle"
	leaseVersion                     = 1
	leaseTtlSeconds                  = 15
	leaseRetrySeconds                = 5
	heartbeatTtlSeconds              = 60 * 60
	hardwareDiscoveryIntervalSeconds = 120 * 60
	watchErrorRetrySeconds           = 2
	machineTtlMinutes                = 5
)

var (
	leaseTtlDuration          = time.Duration(leaseTtlSeconds) * time.Second
	leaseRetryDuration        = time.Duration(leaseRetrySeconds) * time.Second
	machineTtlDuration        = time.Duration(machineTtlMinutes) * time.Minute
	hardwareDiscoveryInterval = time.Duration(hardwareDiscoveryIntervalSeconds) * time.Second
)

// Interface defining how a leader reacts to gaining or losing election, new members being added, etc.
type Leader interface {
	OnLeadershipAcquired() error
	OnLeadershipLost() error
	GetLeaseName() string
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
		return fmt.Errorf("failed to elect leader. %+v", err)
	}

	// initialize the hardware inventory
	err = r.discoverHardware()
	if err != nil {
		return fmt.Errorf("failed to detect initial hardware. %+v", err)
	}

	// in a goroutine, begin the monitor cluster loop for changes in membership, leadership, etc.
	go func() {
		r.refreshLeader()
	}()

	go func() {
		r.discoverHardwareLoop()
	}()

	go func() {
		r.waitForHardwareChangeNotifications()
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
		l, err = r.leaseManager.AcquireLease(r.leader.GetLeaseName(), r.context.NodeID, leaseVersion, leaseTtlDuration)
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
		err = existing.Renew(leaseTtlDuration)
		r.updateLeaderStatus(err)
		return err
	} else if r.isLeader {
		// we used to be the leader, now we are not
		r.isLeader = false
		return r.leader.OnLeadershipLost()
	}

	return nil
}

func (r *ClusterMember) refreshLeader() {
	for {
		// sleep for a portion of the lease TTL and try again
		<-time.After(leaseRetryDuration)

		err := r.ElectLeader()
		if err != nil {
			log.Printf("error while electing leader: %s", err.Error())
		}
	}
}

func (r *ClusterMember) discoverHardwareLoop() {
	for {
		// sleep until it's time to detect hardware again
		// assume the initial discovery was already done
		<-time.After(hardwareDiscoveryInterval)

		err := r.discoverHardware()
		if err != nil {
			log.Printf("error while discovering hardware: %+v", err)
		} else {
			log.Print("hardware discovery complete")
		}
	}
}

func (r *ClusterMember) waitForHardwareChangeNotifications() {
	hardwareTriggerKey := path.Join(inventory.GetNodeConfigKey(r.context.NodeID), inventory.TriggerHardwareDetectionKey)
	hardwareWatcher := r.context.EtcdClient.Watcher(hardwareTriggerKey, nil)
	for {
		// wait for any changes to the hardware detection trigger key
		resp, err := hardwareWatcher.Next(ctx.Background())
		if err != nil {
			if err == ctx.Canceled {
				log.Print("hardware change watching cancelled, bailing out...")
				break
			} else {
				<-time.After(time.Duration(watchErrorRetrySeconds) * time.Second)
				continue
			}
		}

		if resp != nil && resp.Node != nil && resp.Action == store.Set {
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
	machineKey := path.Join(inventory.NodesHealthKey, r.context.NodeID)

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
	key := path.Join(machineKey, inventory.HeartbeatKey)
	_, err = r.context.EtcdClient.Set(ctx.Background(), key, "", &etcd.SetOptions{TTL: inventory.HeartbeatTtlDuration})
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
	return inventory.DiscoverHardware(r.context.NodeID, r.context.EtcdClient, r.context.Executor)
}
