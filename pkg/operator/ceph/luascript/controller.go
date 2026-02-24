/*
Copyright 2026 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package luascript

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/cmd/rook/rook"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/log"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "ceph-lua-script-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "lua-script-controller")

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       reflect.TypeFor[cephv1.CephLuaScript]().Name(),
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileCephLuaScript reconciles a cephLuaScript object
type ReconcileCephLuaScript struct {
	client           client.Client
	scheme           *runtime.Scheme
	context          *clusterd.Context
	clusterSpec      *cephv1.ClusterSpec
	clusterInfo      *cephclient.ClusterInfo
	recorder         events.EventRecorder
	opManagerContext context.Context
}

// Add creates a new cephLuaScript Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context) reconcile.Reconciler {
	return &ReconcileCephLuaScript{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		context:          context,
		recorder:         mgr.GetEventRecorder("rook-" + controllerName),
		opManagerContext: opManagerContext,
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the cephLuaScript CRD object
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephLuaScript{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephLuaScript]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephLuaScript](mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}
	return nil
}

// Reconcile reads that state of the cluster for a cephLuaScript object and makes changes based on the state read
// and what is in the CephLuaScript.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephLuaScript) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, luaScript, err := r.reconcile(request)

	return reporting.ReportReconcileResult(logger, r.recorder, request, &luaScript, reconcileResponse, err)
}

func (r *ReconcileCephLuaScript) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephLuaScript, error) {
	// Fetch the cephLuaScript instance
	cephLuaScript := &cephv1.CephLuaScript{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephLuaScript)
	if err != nil {
		if kerrors.IsNotFound(err) {
			log.NamedDebug(request.NamespacedName, logger, "CephLuaScript resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, *cephLuaScript, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, *cephLuaScript, errors.Wrap(err, "failed to get CephLuaScript")
	}
	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephLuaScript.ObjectMeta.Generation

	// Set a finalizer so we can do cleanup before the object goes away
	generationUpdated, err := opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephLuaScript)
	if err != nil {
		return reconcile.Result{}, *cephLuaScript, errors.Wrap(err, "failed to add finalizer")
	}
	if generationUpdated {
		log.NamedInfo(request.NamespacedName, logger, "reconciling the lua script after adding finalizer")
		return reconcile.Result{}, *cephLuaScript, nil
	}

	// The CR was just created, initializing status fields
	if cephLuaScript.Status == nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.EmptyStatus, nil)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		if !cephLuaScript.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err := opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephLuaScript)
			if err != nil {
				return reconcile.Result{}, *cephLuaScript, errors.Wrap(err, "failed to remove finalizer")
			}
			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephLuaScript, nil
		}
		return reconcileResponse, *cephLuaScript, nil
	}
	r.clusterSpec = &cephCluster.Spec

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, r.clusterSpec)
	if err != nil {
		return reconcile.Result{}, *cephLuaScript, errors.Wrap(err, "failed to populate cluster info")
	}

	// validate the lua script settings
	err = r.validateLuaScriptCR(cephLuaScript)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus, nil)
		return reconcile.Result{}, *cephLuaScript, errors.Wrapf(err, "invalid CephLuaScript CR %q", cephLuaScript.Name)
	}

	// DELETE: the CR was deleted
	if !cephLuaScript.GetDeletionTimestamp().IsZero() {
		res, err := r.deleteCephLuaScript(cephLuaScript)
		return res, *cephLuaScript, err
	}

	// Start object reconciliation, updating status for this
	r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcilingStatus, nil)

	cephObjectStore, err := r.context.RookClientset.CephV1().CephObjectStores(cephLuaScript.Spec.ObjectStoreNamespace).Get(r.clusterInfo.Context, cephLuaScript.Spec.ObjectStoreName, metav1.GetOptions{})
	if err != nil {
		return reconcile.Result{RequeueAfter: 5 * time.Second}, *cephLuaScript, errors.Wrapf(err, "failed to find object store %q", cephLuaScript.Spec.ObjectStoreName)
	}

	zoneName := cephObjectStore.Spec.Zone.Name
	if zoneName == "" {
		zoneName = cephObjectStore.Name
	}
	alreadyManaged, err := r.reconcileLuaScript(cephLuaScript, zoneName)
	if err != nil {
		return reconcile.Result{RequeueAfter: 5 * time.Second}, *cephLuaScript, err
	}
	if alreadyManaged {
		r.updateStatus(observedGeneration, request.NamespacedName, k8sutil.FailedStatus, &zoneName)
		return reconcile.Result{}, *cephLuaScript, nil
	}
	// update ObservedGeneration in status at the end of reconcile
	// Set Ready status, we are done reconciling
	r.updateStatus(observedGeneration, request.NamespacedName, k8sutil.ReadyStatus, &zoneName)

	// Return and do not requeue
	log.NamedDebug(request.NamespacedName, logger, "done reconciling")
	return reconcile.Result{}, *cephLuaScript, nil
}

// Reconcile create/update of the Lua script stored on RGW considering ownership of the script to prevent collisions
// Returns true if cls is being blocked by another managed CephLuaScript and false otherwise
// Returns an error if the reconciler should exit and requeue the cls
func (r *ReconcileCephLuaScript) reconcileLuaScript(cls *cephv1.CephLuaScript, zoneName string) (bool, error) {
	// get the metadata of the script stored in RGW under zoneName
	metadata, err := r.readLuaScriptMetadata(cls, zoneName)
	if err != nil {
		return false, err
	}
	name, namespace, tenant, hash, err := r.parseMetadataComment(metadata)
	if err != nil {
		// there is no metadata or it is corrupted so assume the script is uninitialized
		if err := r.writeLuaScript(cls, zoneName); err != nil {
			return false, err
		}
		return false, nil
	}

	if cls.Name == name && cls.Namespace == namespace && cls.Spec.Tenant == tenant {
		// cls is the owner, check cls' potentially new script against the hash
		scriptContent, err := r.getLuaScriptContent(cls)
		if err != nil {
			return false, err
		}
		if hash != fmt.Sprint(r.hashLuaScript(scriptContent)) {
			// hash doesn't match so a script update is required
			if err := r.writeLuaScript(cls, zoneName); err != nil {
				return false, err
			}
		}
	} else {
		// does the CephLuaScript script owner exist?
		luaScript := &cephv1.CephLuaScript{}
		luaScript.Name = name
		luaScript.Namespace = namespace
		err := r.client.Get(r.opManagerContext, types.NamespacedName{Name: luaScript.Name, Namespace: luaScript.Namespace}, luaScript)
		if err != nil && kerrors.IsNotFound(err) {
			// original CR is not found so cls' script should overwrite it
			if err := r.writeLuaScript(cls, zoneName); err != nil {
				return false, err
			}
		} else if err == nil {
			// cls is blocked by another CephLuaScript that is already being managed
			return true, nil
		}
	}

	return false, nil
}

// Hashes the Lua script content
func (r *ReconcileCephLuaScript) hashLuaScript(scriptContent string) uint64 {
	return xxhash.Sum64String(scriptContent)
}

// Creates a Lua script body annotated based on the CephLuaScript that authored it
func (r *ReconcileCephLuaScript) createLuaScript(cls *cephv1.CephLuaScript) ([]byte, error) {
	scriptContent, err := r.getLuaScriptContent(cls)
	if err != nil {
		return []byte{}, err
	}
	// add a Lua comment marking cls as the author and a hash of the scriptBody
	scriptBody := fmt.Sprintf("-- %s:%s:%s:%d\n", cls.Name, cls.Namespace, cls.Spec.Tenant, r.hashLuaScript(scriptContent))
	scriptBody += scriptContent
	return []byte(scriptBody), nil
}

// Get the Lua script content from one of the CephLuaScript script fields
func (r *ReconcileCephLuaScript) getLuaScriptContent(cls *cephv1.CephLuaScript) (string, error) {
	scriptBody := ""
	// as plaintext
	if cls.Spec.Script != "" {
		scriptBody = cls.Spec.Script
	}
	// or base64 encoded
	if cls.Spec.ScriptBase64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(cls.Spec.ScriptBase64)
		if err != nil {
			return "", err
		}
		scriptBody = string(decoded)
	}
	// or from URL
	if cls.Spec.ScriptURL != "" {
		res, err := http.Get(cls.Spec.ScriptURL)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return "", err
		}
		scriptBody = string(body)
	}
	return scriptBody, nil
}

// Writes a Lua script to the operator's filesystem and returns a string path to the created file
func (r *ReconcileCephLuaScript) createLocalLuaScript(cls *cephv1.CephLuaScript) (string, error) {
	if len(cls.Spec.Script) == 0 {
		return "", nil
	}
	err := os.MkdirAll(cephclient.DefaultWorkingDir, 0o755)
	if err != nil {
		rook.TerminateFatal(errors.Wrapf(err, "failed to mkdir the default working dir at path %q", cephclient.DefaultWorkingDir))
	}
	luaScript, err := r.createLuaScript(cls)
	if err != nil {
		return "", err
	}
	filePath := fmt.Sprintf("%s/%s-%s.lua", cephclient.DefaultWorkingDir, cls.Name, cls.GetObjectMeta().GetUID())
	var fileMode os.FileMode = 0o777
	err = os.WriteFile(filePath, luaScript, fileMode)
	if err != nil {
		rook.TerminateFatal(errors.Wrapf(err, "failed to write lua script to path %q", filePath))
	}
	util.WriteFileToLog(logger, filePath)
	return filePath, nil
}

// Returns the name, namespace, and hash from a metadata comment embedded into the lua script body
func (r *ReconcileCephLuaScript) parseMetadataComment(metadata string) (string, string, string, string, error) {
	name := ""
	namespace := ""
	tenant := ""
	hash := ""
	// find the first comment
	firstCommentIdx := strings.Index(metadata, "--")
	if firstCommentIdx == -1 {
		return name, namespace, tenant, hash, fmt.Errorf("the lua script has no metadata comment")
	}
	// Get the first char after firstCommentIdx
	firstChar := -1
	for i := firstCommentIdx; i < len(metadata); i++ {
		if metadata[i] != ' ' && metadata[i] != '-' {
			firstChar = i
			break
		}
	}
	if firstChar == -1 {
		return name, namespace, tenant, hash, fmt.Errorf("the lua script's metadata comment is corrupted")
	}
	metadata = metadata[firstChar:]
	metadataArr := strings.Split(metadata, ":")
	if len(metadataArr) != 4 {
		return name, namespace, tenant, hash, fmt.Errorf("the lua script metadata comment is invalid; expected length 4 but got %d", len(metadataArr))
	}
	name = metadataArr[0]
	namespace = metadataArr[1]
	tenant = metadataArr[2]
	hash = metadataArr[3]
	return name, namespace, tenant, hash, nil
}

// Deletes the CephLuaScript CR instance
func (r *ReconcileCephLuaScript) deleteCephLuaScript(cls *cephv1.CephLuaScript) (reconcile.Result, error) {
	nsName := opcontroller.NsName(cls.Namespace, cls.Name)
	log.NamedDebug(nsName, logger, "deleting lua script CR %q", cls.Name)

	err := r.deleteLuaScript(cls)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to delete lua script in RGW")
	}

	// Remove finalizer
	err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cls)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
	}

	// Return and do not requeue. Successful deletion.
	return reconcile.Result{}, nil
}

// Deletes the Lua script on RGW using radosgw-admin
func (r *ReconcileCephLuaScript) deleteLuaScript(cls *cephv1.CephLuaScript) error {
	if cls.Status == nil {
		return fmt.Errorf("could not find an object store zone status assigned to CephLuaScript %s", cls.Name)
	}
	zoneName := cls.Status.Zone.Name
	if zoneName == "" {
		// if the status zone name was not set then the operator did not completely reconcile the cls
		// in this case we should try one last time to see if there is an valid ceph object store
		cephObjectStore, err := r.context.RookClientset.CephV1().CephObjectStores(cls.Spec.ObjectStoreNamespace).Get(r.clusterInfo.Context, cls.Spec.ObjectStoreName, metav1.GetOptions{})
		if err != nil {
			// the object store was not found so there is no lua script to remove
			return nil
		}
		zoneName = cephObjectStore.Spec.Zone.Name
		if zoneName == "" {
			zoneName = cephObjectStore.Name
		}
	}

	// check that cls manages the lua script
	metadata, err := r.readLuaScriptMetadata(cls, zoneName)
	if err != nil {
		return err
	}
	name, namespace, tenant, _, err := r.parseMetadataComment(metadata)
	if err != nil {
		// there is no metadata or it is corrupted so assume the script is uninitialized
		// remove the script to reinitialize
	} else if cls.Name != name || cls.Namespace != namespace || cls.Spec.Tenant != tenant {
		// don't clear the script because cls doesn't manage the lua script
		return nil
	}

	objContext := object.NewContext(r.context, r.clusterInfo, zoneName)
	objContext.Realm = zoneName
	objContext.ZoneGroup = zoneName
	objContext.Zone = zoneName

	contextArg := fmt.Sprintf("--context=%s", cls.Spec.Context)
	// zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	args := []string{"script", "rm", contextArg, zoneArg}

	if cls.Spec.Context != cephv1.BackgroundCephLuaScriptContext {
		tenantArg := fmt.Sprintf("--tenant=%s", cls.Spec.Tenant)
		args = append(args, tenantArg)
	}

	output, err := object.RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to remove lua script in zone %q for reason %q", objContext.Zone, output)
	}
	return nil
}

// Gets the Lua script metadata in format "-- <instance>:<namespace>:<tenant>:<content-hash>" or returns an error
func (r *ReconcileCephLuaScript) readLuaScriptMetadata(cls *cephv1.CephLuaScript, zoneName string) (string, error) {
	objContext := object.NewContext(r.context, r.clusterInfo, zoneName)
	objContext.Realm = zoneName
	objContext.ZoneGroup = zoneName
	objContext.Zone = zoneName

	contextArg := fmt.Sprintf("--context=%s", cls.Spec.Context)
	// zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)
	args := []string{"script", "get", contextArg, zoneArg}
	if cls.Spec.Context != cephv1.BackgroundCephLuaScriptContext {
		tenantArg := fmt.Sprintf("--tenant=%s", cls.Spec.Tenant)
		args = append(args, tenantArg)
	}

	output, err := object.RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read lua script in zone %q for reason %q", objContext.Zone, output)
	}
	// get the first line of output
	if strings.Contains(output, "\n") && len(strings.Split(output, "\n")) > 0 {
		output = strings.Split(output, "\n")[0]
	}
	return output, nil
}

// Writes a Lua script into zone zoneName on RGW using radosgw-admin
func (r *ReconcileCephLuaScript) writeLuaScript(cls *cephv1.CephLuaScript, zoneName string) error {
	objContext := object.NewContext(r.context, r.clusterInfo, zoneName)
	objContext.Realm = zoneName
	objContext.ZoneGroup = zoneName
	objContext.Zone = zoneName

	contextArg := fmt.Sprintf("--context=%s", cls.Spec.Context)
	// zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)
	localLuaScript, err := r.createLocalLuaScript(cls)
	if err != nil {
		return err
	}
	infileArg := fmt.Sprintf("--infile=%s", localLuaScript)

	args := []string{"script", "put", contextArg, zoneArg, infileArg}

	if cls.Spec.Context != cephv1.BackgroundCephLuaScriptContext {
		tenantArg := fmt.Sprintf("--tenant=%s", cls.Spec.Tenant)
		args = append(args, tenantArg)
	}

	output, err := object.RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to create lua script in zone %q for reason %q", objContext.Zone, output)
	}
	return nil
}

// validateLuaScriptCR validates the lua script arguments
func (r *ReconcileCephLuaScript) validateLuaScriptCR(cls *cephv1.CephLuaScript) error {
	if cls.Name == "" {
		return errors.New("missing name")
	}
	if cls.Namespace == "" {
		return errors.New("missing namespace")
	}
	hasScript := cls.Spec.Script != ""
	hasScriptBase64 := cls.Spec.ScriptBase64 != ""
	hasScriptURL := cls.Spec.ScriptURL != ""
	scriptCount := 0
	if hasScript {
		scriptCount++
	}
	if hasScriptBase64 {
		scriptCount++
	}
	if hasScriptURL {
		scriptCount++
	}
	if scriptCount == 0 {
		return errors.New("one of script, scriptBase64, or scriptURL must be provided")
	}
	if scriptCount != 1 {
		return errors.New("only one of script, scriptBase64, or scriptURL must be provided")
	}
	if hasScriptURL {
		_, err := url.ParseRequestURI(cls.Spec.ScriptURL)
		if err != nil {
			return errors.Wrapf(err, "scriptURL points to an invalid URL")
		}
	}
	if cls.Spec.Context == cephv1.BackgroundCephLuaScriptContext && cls.Spec.Tenant != "" {
		return errors.New("a tenant name can't be specified when using the background context")
	}
	return nil
}

// updateStatus updates a CephLuaScript with a given status
func (r *ReconcileCephLuaScript) updateStatus(observedGeneration int64, name types.NamespacedName, status string, zoneName *string) {
	luaScript := &cephv1.CephLuaScript{}
	if err := r.client.Get(r.opManagerContext, name, luaScript); err != nil {
		if kerrors.IsNotFound(err) {
			log.NamedDebug(name, logger, "CephLuaScript resource not found. Ignoring since object must be deleted.")
			return
		}
		log.NamedWarning(name, logger, "failed to retrieve object zone %q to update status to %q. %v", name, status, err)
		return
	}
	if luaScript.Status == nil {
		luaScript.Status = &cephv1.LuaScriptStatus{}
	}
	if zoneName != nil {
		luaScript.Status.Zone = cephv1.ZoneSpec{Name: *zoneName}
	}

	luaScript.Status.Phase = status
	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		luaScript.Status.ObservedGeneration = observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, luaScript); err != nil {
		log.NamedError(name, logger, "failed to set object zone %q status to %q. %v", name, status, err)
		return
	}
	log.NamedDebug(name, logger, "object zone %q status updated to %q", name, status)
}
