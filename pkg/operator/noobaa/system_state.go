/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package noobaa

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"text/template"

	dockerref "github.com/docker/distribution/reference"
	semver "github.com/hashicorp/go-version"
	nbv1 "github.com/rook/rook/pkg/apis/noobaa.rook.io/v1alpha1"
	nbclient "github.com/rook/rook/pkg/operator/noobaa/client"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// SystemState keeps the desired/current state of a noobaa system and its resources
type SystemState struct {
	Operator        *Operator
	CachedSystem    *nbv1.NooBaaSystem
	System          *nbv1.NooBaaSystem
	ServiceAccount  *corev1.ServiceAccount
	Role            *rbacv1.Role
	RoleBinding     *rbacv1.RoleBinding
	CoreApp         *appsv1.StatefulSet
	ServiceMgmt     *corev1.Service
	ServiceS3       *corev1.Service
	SecretOp        *corev1.Secret
	SecretAdmin     *corev1.Secret
	NBClient        *nbclient.RPCClient
	ErrorSuppressed bool
}

func NewSystemState(operator *Operator, cachedSys *nbv1.NooBaaSystem) *SystemState {

	sys := cachedSys.DeepCopy()
	serviceAccountName := sys.Name + "-service-account"
	roleName := sys.Name + "-role"
	roleBindingName := sys.Name + "-role-binding"
	coreAppName := sys.Name + "-core"
	ServiceMgmtName := sys.Name + "-mgmt"
	ServiceS3Name := "s3" // TODO: handle collision in namespace
	operatorSecretName := sys.Name + "-operator"
	adminSecretName := sys.Name + "-admin"
	logdirPVCName := sys.Name + "-logdir"
	datadirPVCName := sys.Name + "-datadir"
	corePodSelectorLabels := map[string]string{
		"noobaa-core": coreAppName,
	}
	mgmtSelectorLabels := map[string]string{
		"noobaa-mgmt": coreAppName,
	}
	s3SelectorLabels := map[string]string{
		"noobaa-s3": coreAppName,
	}
	corePodLabels := map[string]string{
		"noobaa-core": coreAppName,
		"noobaa-s3":   coreAppName,
		"noobaa-mgmt": coreAppName,
	}
	prometheusAnnotations := map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/scheme": "http",
		"prometheus.io/port":   "8080",
	}

	if sys.Spec.Image == "" {
		sys.Spec.Image = ContainerImage
	}

	makeInt32Ptr := func(x int32) *int32 { return &x }

	makeObjectMeta := func(name string) metav1.ObjectMeta {
		ownerIsController := true
		owner := metav1.OwnerReference{
			APIVersion: sys.APIVersion,
			Kind:       sys.Kind,
			Name:       sys.GetName(),
			UID:        sys.GetUID(),
			Controller: &ownerIsController,
		}
		return metav1.ObjectMeta{
			Name:            name,
			Namespace:       sys.Namespace,
			OwnerReferences: []metav1.OwnerReference{owner},
			Labels: map[string]string{
				"app": "noobaa",
			},
		}
	}

	addLabels := func(m metav1.ObjectMeta, labels map[string]string) metav1.ObjectMeta {
		m.Labels = addMaps(m.Labels, labels)
		return m
	}

	addAnnotations := func(m metav1.ObjectMeta, annotations map[string]string) metav1.ObjectMeta {
		m.Annotations = addMaps(m.Annotations, annotations)
		return m
	}

	s := &SystemState{
		Operator:     operator,
		CachedSystem: cachedSys,
		System:       sys,

		ServiceAccount: &corev1.ServiceAccount{
			ObjectMeta: makeObjectMeta(serviceAccountName),
		},

		Role: &rbacv1.Role{
			ObjectMeta: makeObjectMeta(roleName),
			// TODO: the goal is to remove all these rules
			// the operator can do what's needed and update the system
			Rules: []rbacv1.PolicyRule{{
				APIGroups: []string{"apps"},
				Resources: []string{"statefulsets"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			}, {
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     []string{"get", "list", "watch"},
			}},
		},

		RoleBinding: &rbacv1.RoleBinding{
			ObjectMeta: makeObjectMeta(roleBindingName),
			Subjects: []rbacv1.Subject{{
				Kind: "ServiceAccount",
				Name: serviceAccountName,
			}},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     roleName,
			},
		},

		SecretOp: &corev1.Secret{
			ObjectMeta: makeObjectMeta(operatorSecretName),
			Type:       corev1.SecretTypeOpaque,
			StringData: map[string]string{},
		},

		SecretAdmin: &corev1.Secret{
			ObjectMeta: makeObjectMeta(adminSecretName),
			Type:       corev1.SecretTypeOpaque,
			StringData: map[string]string{},
		},

		ServiceMgmt: &corev1.Service{
			ObjectMeta: addAnnotations(makeObjectMeta(ServiceMgmtName), prometheusAnnotations),
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeLoadBalancer,
				Selector: mgmtSelectorLabels,
				Ports: []corev1.ServicePort{
					{Port: 8080, Name: "mgmt"},
					{Port: 8443, Name: "mgmt-https"},
					{Port: 8444, Name: "md-https"},
					{Port: 8445, Name: "bg-https"},
					{Port: 8446, Name: "hosted-agents-https"},
					{Port: 80, TargetPort: intstr.FromInt(6001), Name: "s3"},
					{Port: 443, TargetPort: intstr.FromInt(6443), Name: "s3-https"},
				},
			},
		},

		ServiceS3: &corev1.Service{
			ObjectMeta: makeObjectMeta(ServiceS3Name),
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeLoadBalancer,
				Selector: s3SelectorLabels,
				Ports: []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(6001), Name: "s3"},
					{Port: 443, TargetPort: intstr.FromInt(6443), Name: "s3-https"},
				},
			},
		},

		CoreApp: &appsv1.StatefulSet{
			ObjectMeta: makeObjectMeta(coreAppName),
			Spec: appsv1.StatefulSetSpec{
				Replicas: makeInt32Ptr(1),
				Selector: &metav1.LabelSelector{
					MatchLabels: corePodSelectorLabels,
				},
				ServiceName: ServiceMgmtName,
				Template: corev1.PodTemplateSpec{
					ObjectMeta: addLabels(makeObjectMeta(coreAppName), corePodLabels),
					Spec: corev1.PodSpec{
						ServiceAccountName: serviceAccountName,
						Containers: []corev1.Container{{
							Name:            coreAppName,
							Image:           sys.Spec.Image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							ReadinessProbe: &corev1.Probe{
								// # https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-probes
								// # ready when s3 port is open
								TimeoutSeconds: 5,
								Handler: corev1.Handler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(6001),
									},
								},
							},
							Resources: corev1.ResourceRequirements{
								// # https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
								},
							},
							Ports: []corev1.ContainerPort{
								{ContainerPort: 80},
								{ContainerPort: 443},
								{ContainerPort: 8080},
								{ContainerPort: 8443},
								{ContainerPort: 8444},
								{ContainerPort: 8445},
								{ContainerPort: 8446},
								{ContainerPort: 60100},
							},
							VolumeMounts: []corev1.VolumeMount{
								{MountPath: "/data", Name: datadirPVCName},
								{MountPath: "/log", Name: logdirPVCName},
							},
							Env: []corev1.EnvVar{
								{Name: "CONTAINER_PLATFORM", Value: "KUBERNETES"},
							},
						}},
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: makeObjectMeta(datadirPVCName),
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("50Gi"),
								},
							},
						},
					}, {
						ObjectMeta: makeObjectMeta(logdirPVCName),
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("10Gi"),
								},
							},
						},
					},
				},
			},
		},
	}
	return s
}

func (s *SystemState) Sync() error {
	var err error

	err = s.SyncSystemSpec()
	if err != nil {
		return err
	}

	err = s.Operator.SyncServiceAccount(s.ServiceAccount)
	if err != nil {
		return err
	}
	err = s.Operator.SyncRole(s.Role)
	if err != nil {
		return err
	}
	err = s.Operator.SyncRoleBinding(s.RoleBinding)
	if err != nil {
		return err
	}
	err = s.Operator.SyncStatefulSet(s.CoreApp)
	if err != nil {
		return err
	}
	err = s.Operator.SyncService(s.ServiceMgmt)
	if err != nil {
		return err
	}
	err = s.Operator.SyncService(s.ServiceS3)
	if err != nil {
		return err
	}

	svcStatus := &s.System.Status.Services
	s.SyncServiceStatus(s.ServiceMgmt, &svcStatus.ServiceMgmt, "mgmt-https")
	s.SyncServiceStatus(s.ServiceS3, &svcStatus.ServiceS3, "s3-https")

	err = s.SyncNBClient()
	if err != nil {
		return err
	}
	err = s.SyncSystemInNooBaa()
	if err != nil {
		return err
	}
	err = s.SyncSecretAdmin()
	if err != nil {
		return err
	}

	err = s.SyncSystemStatus()
	if err != nil {
		return err
	}

	return nil
}

func (s *SystemState) SyncSystemSpec() error {

	imageRef, err := dockerref.Parse(s.System.Spec.Image)
	if err != nil {
		return err
	}
	imageName := ""
	imageTag := ""
	switch image := imageRef.(type) {
	case dockerref.NamedTagged:
		logger.Infof("System: SyncSystemSpec() NamedTagged %#v", image)
		imageName = image.Name()
		imageTag = image.Tag()
	case dockerref.Tagged:
		logger.Infof("System: SyncSystemSpec() Tagged %#v", image)
		imageTag = image.Tag()
	case dockerref.Named:
		logger.Infof("System: SyncSystemSpec() Named %#v", image)
		imageName = image.Name()
	default:
		logger.Infof("System: SyncSystemSpec() Ref %#v", image)
	}

	if imageName == ContainerImageName {
		version, err := semver.NewVersion(imageTag)
		if err == nil {
			logger.Infof("System: SyncSystemSpec() semver %s", version.String())
			if !ContainerImageConstraint.Check(version) {
				logger.Errorf("System: SyncSystemSpec() Unsupported image version %+v %+v", imageRef, ContainerImageConstraint)
				s.Operator.EventRecorder.Eventf(s.System, corev1.EventTypeWarning, "BadImage",
					"NooBaa System image requested unsupported version %s not matching constraints %s", imageRef, ContainerImageConstraint.String())
				s.ErrorSuppressed = true
				return fmt.Errorf("Unsupported image version %+v", imageRef)
			}
		}
	} else {
		logger.Infof("System: SyncSystemSpec() non-standard image name %s not matching default %s", imageRef, ContainerImageName)
		s.Operator.EventRecorder.Eventf(s.System, corev1.EventTypeNormal, "NonStandardImage",
			"NooBaa System image requested non-standard image name %s not matching default %s", imageRef, ContainerImageName)
	}

	if s.System.Spec.Image == s.CachedSystem.Spec.Image {
		return nil
	}

	specPatch := JSONDict{"spec": JSONDict{"image": s.System.Spec.Image}}
	client := s.Operator.RookClient.NoobaaV1alpha1().NooBaaSystems(s.System.Namespace)
	_, err = client.Patch(s.System.Name, types.MergePatchType, MakeJSON(specPatch))
	if err != nil {
		logger.Errorf("System: SyncSystemSpec() patch failed %s", err)
	}

	return err
}

func (s *SystemState) SyncServiceStatus(
	srv *corev1.Service,
	status *nbv1.SystemServiceStatus,
	portName string,
) {
	*status = nbv1.SystemServiceStatus{}
	servicePort := nbclient.FindPortByName(srv, portName)
	proto := "http"
	if strings.HasSuffix(portName, "https") {
		proto = "https"
	}

	// Node IP:Port

	nodes, err := s.Operator.NodeInformer.Lister().List(labels.Everything())
	if err == nil {
		for _, node := range nodes {
			for _, addr := range node.Status.Addresses {
				switch addr.Type {
				case corev1.NodeHostName:
					break // currently ignoring
				case corev1.NodeExternalIP:
					fallthrough
				case corev1.NodeExternalDNS:
					fallthrough
				case corev1.NodeInternalIP:
					fallthrough
				case corev1.NodeInternalDNS:
					logger.Tracef("System: SyncServiceStatus() adding NodePorts %+v", addr)
					status.NodePorts = append(
						status.NodePorts,
						fmt.Sprintf("%s://%s:%d", proto, addr.Address, servicePort.NodePort),
					)
				}

			}
		}
	}

	// Pod IP:Port

	pods, err := s.Operator.PodInformer.Lister().List(labels.SelectorFromSet(srv.Spec.Selector))
	if err == nil {
		for _, pod := range pods {
			if pod.Status.PodIP != "" {
				status.PodPorts = append(
					status.PodPorts,
					fmt.Sprintf("%s://%s:%s", proto, pod.Status.PodIP, servicePort.TargetPort.String()),
				)
			}
		}
	}

	// Cluster IP:Port (of the service)

	if srv.Spec.ClusterIP != "" {
		status.InternalIP = append(
			status.InternalIP,
			fmt.Sprintf("%s://%s:%d", proto, srv.Spec.ClusterIP, servicePort.Port),
		)
		status.InternalDNS = append(
			status.InternalDNS,
			fmt.Sprintf("%s://%s.%s:%d", proto, srv.Name, srv.Namespace, servicePort.Port),
		)
	}

	// LoadBalancer IP:Port (of the service)

	if srv.Status.LoadBalancer.Ingress != nil {
		for _, lb := range srv.Status.LoadBalancer.Ingress {
			if lb.IP != "" {
				status.ExternalIP = append(
					status.ExternalIP,
					fmt.Sprintf("%s://%s:%d", proto, lb.IP, servicePort.Port),
				)
			}
			if lb.Hostname != "" {
				status.ExternalDNS = append(
					status.ExternalDNS,
					fmt.Sprintf("%s://%s:%d", proto, lb.Hostname, servicePort.Port),
				)
			}
		}
	}

	// External IP:Port (of the service)

	if srv.Spec.ExternalIPs != nil {
		for _, ip := range srv.Spec.ExternalIPs {
			status.ExternalIP = append(
				status.ExternalIP,
				fmt.Sprintf("%s://%s:%d", proto, ip, servicePort.Port),
			)
		}
	}

	logger.Infof("System: SyncServiceStatus() collected addresses for %s/%s: %+v", srv.Namespace, srv.Name, status)
}

func (s *SystemState) SyncNBClient() error {

	if len(s.System.Status.Services.ServiceMgmt.PodPorts) == 0 {
		logger.Infof("System: SyncNBClient() core pod not ready %s", s.System.Name)
		return fmt.Errorf("core pod not ready")
	}

	if s.Operator.DevMode {
		nodePort := s.System.Status.Services.ServiceMgmt.NodePorts[0]
		nodeIP := nodePort[strings.Index(nodePort, "://")+3 : strings.LastIndex(nodePort, ":")]
		s.NBClient = nbclient.NewClient(&nbclient.RPCRouterNodePort{
			ServiceMgmt: s.ServiceMgmt,
			NodeIP:      nodeIP,
		})
	} else {
		podPort := s.System.Status.Services.ServiceMgmt.PodPorts[0]
		podIP := podPort[strings.Index(podPort, "://")+3 : strings.LastIndex(podPort, ":")]
		s.NBClient = nbclient.NewClient(&nbclient.RPCRouterPodPort{
			ServiceMgmt: s.ServiceMgmt,
			PodIP:       podIP,
		})
	}
	return nil
}

func (s *SystemState) SyncSystemInNooBaa() error {

	// Note: the operator secret is created in the system namespace
	lister := s.Operator.SecretInformer.Lister().Secrets(s.SecretOp.Namespace)
	client := s.Operator.KubeClient.CoreV1().Secrets(s.SecretOp.Namespace)

	secretOp, err := lister.Get(s.SecretOp.Name)
	if err == nil {
		s.SecretOp = secretOp.DeepCopy()
		return nil
	}
	if !errors.IsNotFound(err) {
		logger.Errorf("System: SyncSystemInNooBaa() Error getting secret %s", err)
		return err
	}

	randomBytes := make([]byte, 16)
	_, err = rand.Read(randomBytes)
	if err != nil {
		return err
	}
	randomPassword := base64.StdEncoding.EncodeToString(randomBytes)
	email := "admin@noobaa.io"

	logger.Infof("System: CreateSystemInNooBaa() create system")
	req := &nbclient.CreateSystemAPI{
		Name:     s.System.Name,
		Email:    email,
		Password: randomPassword,
	}
	err = s.NBClient.Call(req)
	if err != nil {
		return err
	}

	s.NBClient.AuthToken = req.Response.Reply.Token
	logger.Infof("System: CreateSystemInNooBaa() response %+v", req.Response)

	s.SecretOp.StringData["system"] = s.System.Name
	s.SecretOp.StringData["email"] = email
	s.SecretOp.StringData["password"] = randomPassword
	s.SecretOp.StringData["auth_token"] = req.Response.Reply.Token

	s.SecretAdmin.StringData["system"] = s.System.Name
	s.SecretAdmin.StringData["email"] = email
	s.SecretAdmin.StringData["password"] = randomPassword

	secretOp, err = client.Create(s.SecretOp)
	if err != nil {
		return err
	}
	s.SecretOp = secretOp
	return nil
}

func (s *SystemState) SyncSecretAdmin() error {

	lister := s.Operator.SecretInformer.Lister().Secrets(s.SecretAdmin.Namespace)
	client := s.Operator.KubeClient.CoreV1().Secrets(s.SecretAdmin.Namespace)

	secretAdmin, err := lister.Get(s.SecretAdmin.Name)
	if err == nil {
		s.SecretAdmin = secretAdmin.DeepCopy()
		return nil
	}
	if !errors.IsNotFound(err) {
		logger.Errorf("System: SyncSecretAdmin() Error getting secret %s", err)
		return err
	}

	logger.Infof("System: SyncAdminS3Credentials() list accounts")
	req := &nbclient.ListAccountsAPI{}
	err = s.NBClient.Call(req)
	if err != nil {
		return err
	}
	for _, account := range req.Response.Reply.Accounts {
		if account.Email == "admin@noobaa.io" {
			if len(account.AccessKeys) > 0 {
				s.SecretAdmin.StringData["AWS_ACCESS_KEY_ID"] = account.AccessKeys[0].AccessKey
				s.SecretAdmin.StringData["AWS_SECRET_ACCESS_KEY"] = account.AccessKeys[0].SecretKey
			}
		}
	}

	secretAdmin, err = client.Create(s.SecretAdmin)
	if err != nil {
		return err
	}
	s.SecretAdmin = secretAdmin
	return nil
}

var readmeTemplate = template.Must(template.New("NooBaaSystem.Status.Readme").Parse(`

	Welcome to NooBaa!
	-----------------

	Lets get started:

	1. Connect to Management console:

		Read your mgmt console login information (email & password) from secret: "{{.SecretAdmin.Name}}".
	
			kubectl get secret {{.SecretAdmin.Name}} -n {{.SecretAdmin.Namespace}} -o json | jq '.data|map_values(@base64d)'

		Open the management console service - take External IP/DNS or Node Port or use port forwarding:

			kubectl port-forward -n {{.ServiceMgmt.Namespace}} service/{{.ServiceMgmt.Name}} 11443:8443 &
			open https://localhost:11443

	2. Test S3 client:

		kubectl port-forward -n {{.ServiceS3.Namespace}} service/{{.ServiceS3.Name}} 10443:443 &
		NOOBAA_ACCESS_KEY=$(kubectl get secret {{.SecretAdmin.Name}} -n {{.SecretAdmin.Namespace}} -o json | jq -r '.data.AWS_ACCESS_KEY_ID|@base64d')
		NOOBAA_SECRET_KEY=$(kubectl get secret {{.SecretAdmin.Name}} -n {{.SecretAdmin.Namespace}} -o json | jq -r '.data.AWS_SECRET_ACCESS_KEY|@base64d')
		alias s3='AWS_ACCESS_KEY_ID=$NOOBAA_ACCESS_KEY AWS_SECRET_ACCESS_KEY=$NOOBAA_SECRET_KEY aws --endpoint https://localhost:10443 --no-verify-ssl s3'
		s3 ls

`))

func (s *SystemState) SyncSystemStatus() error {

	var readmeBuffer bytes.Buffer
	err := readmeTemplate.Execute(&readmeBuffer, s)
	if err != nil {
		return err
	}
	s.System.Status.Readme = readmeBuffer.String()
	s.System.Status.Accounts.Admin.SecretRef.Name = s.SecretAdmin.Name
	s.System.Status.Accounts.Admin.SecretRef.Namespace = s.SecretAdmin.Namespace

	client := s.Operator.RookClient.NoobaaV1alpha1().NooBaaSystems(s.System.Namespace)
	result, err := client.UpdateStatus(s.System)
	if err != nil {
		return err
	}

	s.System = result
	return nil
}

func addMaps(m map[string]string, more map[string]string) map[string]string {
	if m == nil {
		t := m
		m = more
		more = t
	}
	for k, v := range more {
		m[k] = v
	}
	return m
}
