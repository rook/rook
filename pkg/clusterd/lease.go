package clusterd

import (
	"encoding/json"
	"path"
	"time"

	etcd "github.com/coreos/etcd/client"

	"golang.org/x/net/context"
)

const (
	EtcdRequestTimeout = time.Second
	LeaderKey          = "/rook/leader"
	leasePrefix        = "lease"
)

// The interfaces and implementation for leasing found in this source file comes from
// https://github.com/coreos/fleet/tree/master/pkg/lease
// We cannot reference the fleet package due to the fleet reference to the etcdclient
// as part of the antiquated godeps _workspace. With the move to the vendor folder,
// we were required to break this indirect dependency and move the source code to our
// own repo.
type Lease interface {
	Renew(time.Duration) error
	Release() error
	MachineID() string
	Version() int
	Index() uint64
	TimeRemaining() time.Duration
}

type Manager interface {
	GetLease(name string) (Lease, error)
	AcquireLease(name, machID string, ver int, period time.Duration) (Lease, error)
	StealLease(name, machID string, ver int, period time.Duration, idx uint64) (Lease, error)
}

func initLeaseManager(etcdClient etcd.KeysAPI) (Manager, error) {
	return newEtcdLeaseManager(etcdClient, LeaderKey, EtcdRequestTimeout), nil
}

type etcdLeaseMetadata struct {
	MachineID string
	Version   int
}

type lease struct {
	mgr  *leaseManager
	key  string
	meta etcdLeaseMetadata
	idx  uint64
	ttl  time.Duration
}

func (l *lease) Release() error {
	opts := &etcd.DeleteOptions{PrevIndex: l.idx}
	_, err := l.mgr.etcdClient.Delete(context.Background(), l.key, opts)
	return err
}

func (l *lease) Renew(period time.Duration) error {
	val, err := serializeLeaseMetadata(l.meta.MachineID, l.meta.Version)
	opts := &etcd.SetOptions{PrevIndex: l.idx, TTL: period}
	resp, err := l.mgr.etcdClient.Set(context.Background(), l.key, val, opts)
	if err != nil {
		return err
	}

	renewed := l.mgr.leaseFromResponse(resp)
	*l = *renewed

	return nil
}

func (l *lease) MachineID() string {
	return l.meta.MachineID
}

func (l *lease) Version() int {
	return l.meta.Version
}

func (l *lease) Index() uint64 {
	return l.idx
}

func (l *lease) TimeRemaining() time.Duration {
	return l.ttl
}

func serializeLeaseMetadata(machID string, ver int) (string, error) {
	meta := etcdLeaseMetadata{
		MachineID: machID,
		Version:   ver,
	}

	b, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

type leaseManager struct {
	etcdClient etcd.KeysAPI
	keyPrefix  string
	reqTimeout time.Duration
}

func newEtcdLeaseManager(etcdClient etcd.KeysAPI, keyPrefix string, reqTimeout time.Duration) Manager {
	return &leaseManager{etcdClient: etcdClient, keyPrefix: keyPrefix, reqTimeout: reqTimeout}
}

func (r *leaseManager) ctx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), r.reqTimeout)
	return ctx
}

func (r *leaseManager) leasePath(name string) string {
	return path.Join(r.keyPrefix, leasePrefix, name)
}

func (r *leaseManager) GetLease(name string) (Lease, error) {
	key := r.leasePath(name)
	resp, err := r.etcdClient.Get(r.ctx(), key, nil)
	if err != nil {
		if isEtcdError(err, etcd.ErrorCodeKeyNotFound) {
			err = nil
		}
		return nil, err
	}

	l := r.leaseFromResponse(resp)
	return l, nil
}

func (r *leaseManager) StealLease(name, machID string, ver int, period time.Duration, idx uint64) (Lease, error) {
	val, err := serializeLeaseMetadata(machID, ver)
	if err != nil {
		return nil, err
	}

	key := r.leasePath(name)
	opts := &etcd.SetOptions{
		PrevIndex: idx,
		TTL:       period,
	}
	resp, err := r.etcdClient.Set(r.ctx(), key, val, opts)
	if err != nil {
		if isEtcdError(err, etcd.ErrorCodeNodeExist) {
			err = nil
		}
		return nil, err
	}

	l := r.leaseFromResponse(resp)
	return l, nil
}

func (r *leaseManager) AcquireLease(name string, machID string, ver int, period time.Duration) (Lease, error) {
	val, err := serializeLeaseMetadata(machID, ver)
	if err != nil {
		return nil, err
	}

	key := r.leasePath(name)
	opts := &etcd.SetOptions{
		TTL:       period,
		PrevExist: etcd.PrevNoExist,
	}

	resp, err := r.etcdClient.Set(r.ctx(), key, val, opts)
	if err != nil {
		if isEtcdError(err, etcd.ErrorCodeNodeExist) {
			err = nil
		}
		return nil, err
	}

	l := r.leaseFromResponse(resp)
	return l, nil
}

func (r *leaseManager) leaseFromResponse(res *etcd.Response) *lease {
	l := &lease{
		mgr: r,
		key: res.Node.Key,
		idx: res.Node.ModifiedIndex,
		ttl: res.Node.TTLDuration(),
	}

	err := json.Unmarshal([]byte(res.Node.Value), &l.meta)

	// fall back to using the entire value as the MachineID for
	// backwards-compatibility with engines that are not aware
	// of this versioning mechanism
	if err != nil {
		l.meta = etcdLeaseMetadata{
			MachineID: res.Node.Value,
			Version:   0,
		}
	}

	return l
}

func isEtcdError(err error, code int) bool {
	eerr, ok := err.(etcd.Error)
	return ok && eerr.Code == code
}
