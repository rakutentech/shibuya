package scheduler

import (
	e "errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/harpratap/shibuya/config"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	metricsc "k8s.io/metrics/pkg/client/clientset/versioned"
)

func GenerateName(name string, planID int64, collectionID int64, projectID int64, engineNo int) string {
	return fmt.Sprintf("%s-%d-%d-%d-%d", name, projectID, collectionID, planID, engineNo)
}

type K8sClientManager struct {
	*config.ExecutorConfig
	client         *kubernetes.Clientset
	metricClient   *metricsc.Clientset
	serviceAccount string
}

func NewK8sClientManager() *K8sClientManager {
	c, err := config.GetKubeClient()
	if err != nil {
		log.Warning(err)
	}
	metricsc, err := config.GetMetricsClient()
	return &K8sClientManager{
		config.SC.ExecutorConfig, c, metricsc, "shibuya-ingress-serviceaccount",
	}

}

func makeNodeAffinity(key, value string) *apiv1.NodeAffinity {
	nodeAffinity := &apiv1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
			NodeSelectorTerms: []apiv1.NodeSelectorTerm{
				{
					MatchExpressions: []apiv1.NodeSelectorRequirement{
						{
							Key:      key,
							Operator: apiv1.NodeSelectorOpIn,
							Values: []string{
								value,
							},
						},
					},
				},
			},
		},
	}
	return nodeAffinity
}

func collectionNodeAffinity(collectionID int64) *apiv1.NodeAffinity {
	collectionIDStr := fmt.Sprintf("%d", collectionID)
	return makeNodeAffinity("collection_id", collectionIDStr)
}

func prepareAffinity(collectionID int64) *apiv1.Affinity {
	affinity := &apiv1.Affinity{}
	if config.SC.ExecutorConfig.Cluster.OnDemand {
		affinity.NodeAffinity = collectionNodeAffinity(collectionID)
		return affinity
	}
	na := config.SC.ExecutorConfig.NodeAffinity
	if len(na) > 0 {
		t := na[0]
		affinity.NodeAffinity = makeNodeAffinity(t["key"], t["value"])
		return affinity
	}
	return affinity
}

func makeBaseLabel(collectionID, projectID int64) map[string]string {
	return map[string]string{
		"collection": strconv.FormatInt(collectionID, 10),
		"project":    strconv.FormatInt(projectID, 10),
	}
}
func makeIngressLabel(collectionID, projectID int64) map[string]string {
	base := makeBaseLabel(collectionID, projectID)
	base["kind"] = "ingress-controller"
	return base
}

func makeEngineLabel(planID, collectionID, projectID int64, app string) map[string]string {
	base := makeBaseLabel(collectionID, projectID)
	base["app"] = app
	base["plan"] = strconv.FormatInt(planID, 10)
	base["kind"] = "executor"
	return base
}

func (kcm *K8sClientManager) makeHostAliases() []apiv1.HostAlias {
	if kcm.HostAliases != nil {
		hostAliases := []apiv1.HostAlias{}
		for _, ha := range kcm.HostAliases {
			hostAliases = append(hostAliases, apiv1.HostAlias{
				Hostnames: []string{ha.Hostname},
				IP:        ha.IP,
			})
		}
		return hostAliases
	}
	return []apiv1.HostAlias{}
}

func (kcm *K8sClientManager) generateEngineDeployment(replicas int32, name string, planID int64, collectionID int64,
	projectID int64, containerConfig *config.ExecutorContainer) appsv1.Deployment {
	affinity := prepareAffinity(collectionID)
	t := true
	labels := makeEngineLabel(planID, collectionID, projectID, name)
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:                       name,
			DeletionGracePeriodSeconds: new(int64),
			Labels:                     labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: apiv1.PodSpec{
					Affinity:                     affinity,
					ServiceAccountName:           kcm.serviceAccount,
					AutomountServiceAccountToken: &t,
					ImagePullSecrets: []apiv1.LocalObjectReference{
						{
							Name: kcm.ImagePullSecret,
						},
					},
					TerminationGracePeriodSeconds: new(int64),
					HostAliases:                   kcm.makeHostAliases(),
					Containers: []apiv1.Container{
						{
							Name:            name,
							Image:           containerConfig.Image,
							ImagePullPolicy: kcm.ImagePullPolicy,
							Resources: apiv1.ResourceRequirements{
								Limits: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse(containerConfig.CPU),
									apiv1.ResourceMemory: resource.MustParse(containerConfig.Mem),
								},
								Requests: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse(containerConfig.CPU),
									apiv1.ResourceMemory: resource.MustParse(containerConfig.Mem),
								},
							},
							Ports: []apiv1.ContainerPort{
								{
									Name:          "http",
									Protocol:      apiv1.ProtocolTCP,
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
		},
	}
	return deployment
}

func (kcm *K8sClientManager) deploy(deployment *appsv1.Deployment) error {
	deploymentsClient := kcm.client.AppsV1().Deployments(kcm.Namespace)
	_, err := deploymentsClient.Create(deployment)
	if errors.IsAlreadyExists(err) {
		// do nothing if already exists
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (kcm *K8sClientManager) expose(name string, deployment *appsv1.Deployment) error {
	service := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				"networking.istio.io/exportTo": ".",
			},
			Labels: deployment.Spec.Template.ObjectMeta.Labels,
		},
		Spec: apiv1.ServiceSpec{
			Selector: deployment.Spec.Template.ObjectMeta.Labels,
			Ports: []apiv1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromString("http"),
				},
			},
		},
	}
	if deployment.Labels["kind"] == "ingress-controller" {
		if !kcm.InCluster {
			service.Spec.Type = apiv1.ServiceTypeNodePort
		}
		if kcm.Cluster.OnDemand {
			service.Spec.ExternalTrafficPolicy = "Local"
			service.Spec.Type = apiv1.ServiceTypeLoadBalancer
		}
	}
	_, err := kcm.client.CoreV1().Services(kcm.Namespace).Create(service)
	if errors.IsAlreadyExists(err) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (kcm *K8sClientManager) getRandomHostIP() (string, error) {
	podList, err := kcm.client.CoreV1().Pods(kcm.Namespace).
		List(metav1.ListOptions{
			Limit: 1,
		})
	if err != nil {
		log.Error(err)
		return "", err
	}
	if len(podList.Items) == 0 {
		return "", e.New("No pods in Namespace")
	} else {
		return podList.Items[0].Status.HostIP, nil
	}
}

func (kcm *K8sClientManager) CreateService(serviceName string, engine appsv1.Deployment) error {
	err := kcm.expose(serviceName, &engine)
	if err != nil {
		log.Error(err)
		return err
	}
	return nil
}

func (kcm *K8sClientManager) DeployEngine(engineName, serviceName, ingressClass, ingressName string,
	planID, collectionID, projectID int64, containerConfig *config.ExecutorContainer) error {
	engineConfig := kcm.generateEngineDeployment(1, engineName, planID, collectionID, projectID, containerConfig)
	err := kcm.deploy(&engineConfig)
	if err != nil {
		log.Error(err)
		return err
	}
	err = kcm.CreateService(serviceName, engineConfig)
	if err != nil {
		log.Error(err)
		return err
	}
	err = kcm.CreateIngress(ingressClass, ingressName, serviceName, collectionID, projectID)
	if err != nil {
		log.Error(err)
		return err
	}
	log.Printf("Finish creating one engine for %s", engineName)
	return nil
}

func (kcm *K8sClientManager) GetIngressUrl(igName string) (string, error) {
	serviceClient, err := kcm.client.CoreV1().Services(kcm.Namespace).
		Get(igName, metav1.GetOptions{})
	if err != nil {
		return "", makeSchedulerIngressError(err)
	}
	if kcm.InCluster {
		return serviceClient.Spec.ClusterIP, nil
	}
	if kcm.Cluster.OnDemand {
		// in case of GCP getting public IP is enough since it exposes to port 80
		if len(serviceClient.Status.LoadBalancer.Ingress) == 0 {
			return "", makeIPNotAssignedError()
		}
		return serviceClient.Status.LoadBalancer.Ingress[0].IP, nil
	}
	ip_addr, err := kcm.getRandomHostIP()
	if err != nil {
		return "", makeSchedulerIngressError(err)
	}
	exposedPort := serviceClient.Spec.Ports[0].NodePort
	return fmt.Sprintf("%s:%d", ip_addr, exposedPort), nil
}

func (kcm *K8sClientManager) GetPods(labelSelector, fieldSelector string) ([]apiv1.Pod, error) {
	podsClient, err := kcm.client.CoreV1().Pods(kcm.Namespace).
		List(metav1.ListOptions{
			LabelSelector: labelSelector,
			FieldSelector: fieldSelector,
		})
	if err != nil {
		return nil, err
	}
	return podsClient.Items, nil
}

func (kcm *K8sClientManager) GetPodsByCollection(collectionID int64, fieldSelector string) []apiv1.Pod {
	labelSelector := fmt.Sprintf("collection=%d", collectionID)
	pods, err := kcm.GetPods(labelSelector, fieldSelector)
	if err != nil {
		log.Warn(err)
	}
	return pods
}

func (kcm *K8sClientManager) GetPodsByCollectionPlan(collectionID, planID int64) ([]apiv1.Pod, error) {
	labelSelector := fmt.Sprintf("plan=%d,collection=%d", planID, collectionID)
	fieldSelector := ""
	return kcm.GetPods(labelSelector, fieldSelector)
}

func (kcm *K8sClientManager) FetchLogFromPod(pod apiv1.Pod) (string, error) {
	logOptions := &apiv1.PodLogOptions{
		Follow: false,
	}
	req := kcm.client.CoreV1().RESTClient().Get().
		Namespace(pod.Namespace).
		Name(pod.Name).
		Resource("pods").
		SubResource("log").
		Param("follow", strconv.FormatBool(logOptions.Follow)).
		Param("container", logOptions.Container).
		Param("previous", strconv.FormatBool(logOptions.Previous)).
		Param("timestamps", strconv.FormatBool(logOptions.Timestamps))
	readCloser, err := req.Stream()
	if err != nil {
		return "", err
	}
	defer readCloser.Close()
	c, err := ioutil.ReadAll(readCloser)
	if err != nil {
		return "", err
	}
	return string(c), nil
}

func (kcm *K8sClientManager) DownloadPodLog(collectionID, planID int64) (string, error) {
	pods, err := kcm.GetPodsByCollectionPlan(collectionID, planID)
	if err != nil {
		return "", err
	}
	if len(pods) > 0 {
		return kcm.FetchLogFromPod(pods[0])
	}
	return "", fmt.Errorf("Cannot find pod for the plan %d", planID)
}

func (kcm *K8sClientManager) PodReady(label string) int {
	podsClient, err := kcm.client.CoreV1().Pods(kcm.Namespace).
		List(metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s", label),
		})
	if err != nil {
		log.Warn(err)
	}
	ready := 0
	for _, pod := range podsClient.Items {
		if pod.Status.Phase == "Running" {
			ready++
		}
	}
	return ready
}

func (kcm *K8sClientManager) ServiceReachable(ingressClass, serviceName string) bool {
	ingressUrl, err := kcm.GetIngressUrl(ingressClass)
	if err != nil {
		return false
	}
	resp, err := http.Get(fmt.Sprintf("http://%s/%s/start", ingressUrl, serviceName))
	if err != nil {
		log.Warn(err)
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (kcm *K8sClientManager) deleteService(collectionID int64) error {
	// Delete services by collection is not supported as of yet
	// Wait for this PR to be merged - https://github.com/kubernetes/kubernetes/pull/85802
	cmd := exec.Command("kubectl", "-n", kcm.Namespace, "delete", "svc", "--force", "--grace-period=0", "-l", fmt.Sprintf("collection=%d", collectionID))
	o, err := cmd.Output()
	if err != nil {
		log.Printf("Cannot delete services for collection %d", collectionID)
		return err
	}
	log.Print(string(o))
	return nil
}

func (kcm *K8sClientManager) deleteDeployment(collectionID int64) error {
	deploymentsClient := kcm.client.AppsV1().Deployments(kcm.Namespace)
	err := deploymentsClient.DeleteCollection(&metav1.DeleteOptions{
		GracePeriodSeconds: new(int64),
	}, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("collection=%d", collectionID),
	})
	if err != nil {
		log.Error(err)
		return err
	}
	return nil
}

func (kcm *K8sClientManager) PurgeCollection(collectionID int64) error {
	err := kcm.deleteDeployment(collectionID)
	if err != nil {
		return err
	}
	err = kcm.deleteService(collectionID)
	if err != nil {
		return err
	}
	err = kcm.deleteIngressRules(collectionID)
	if err != nil {
		return err
	}
	return nil
}

func int32Ptr(i int32) *int32 { return &i }

func (kcm *K8sClientManager) generateControllerDeployment(igName string, collectionID, projectID int64) appsv1.Deployment {
	publishService := fmt.Sprintf("--publish-service=$(POD_NAMESPACE)/%s", igName)
	affinity := prepareAffinity(collectionID)
	t := true
	labels := makeIngressLabel(collectionID, projectID)
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   igName,
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"prometheus.io/port":   "10254",
						"prometheus.io/scrape": "true",
					},
				},
				Spec: apiv1.PodSpec{
					Affinity:                      affinity,
					ServiceAccountName:            kcm.serviceAccount,
					TerminationGracePeriodSeconds: new(int64),
					AutomountServiceAccountToken:  &t,
					Containers: []apiv1.Container{
						{
							Name:  "nginx-ingress-controller",
							Image: config.SC.IngressConfig.Image,
							Args: []string{
								"/nginx-ingress-controller",
								fmt.Sprintf("--ingress-class=%s", igName),
								"--configmap=$(POD_NAMESPACE)/nginx-configuration",
								publishService,
								"--annotations-prefix=nginx.ingress.kubernetes.io",
								fmt.Sprintf("--watch-namespace=%s", kcm.Namespace),
							},
							SecurityContext: &apiv1.SecurityContext{
								Capabilities: &apiv1.Capabilities{
									Drop: []apiv1.Capability{
										"ALL",
									},
									Add: []apiv1.Capability{
										"NET_BIND_SERVICE",
									},
								},
							},
							Ports: []apiv1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 80,
								},
								{
									Name:          "https",
									ContainerPort: 443,
								},
							},
							Env: []apiv1.EnvVar{
								apiv1.EnvVar{
									Name: "POD_NAME",
									ValueFrom: &apiv1.EnvVarSource{
										FieldRef: &apiv1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								apiv1.EnvVar{
									Name: "POD_NAMESPACE",
									ValueFrom: &apiv1.EnvVarSource{
										FieldRef: &apiv1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return deployment
}

func (kcm *K8sClientManager) DeployIngressController(igName string, collectionID, projectID int64) (string, error) {
	deployment := kcm.generateControllerDeployment(igName, collectionID, projectID)
	err := kcm.deploy(&deployment)
	if err != nil {
		return "", err
	}
	err = kcm.expose(igName, &deployment)
	if err != nil {
		return "", err
	}
	return "", nil
}

func (kcm *K8sClientManager) CreateIngress(ingressClass, ingressName, serviceName string, collectionID, projectID int64) error {
	ingressRule := extbeta1.IngressRule{}
	ingressRule.HTTP = &extbeta1.HTTPIngressRuleValue{
		Paths: []extbeta1.HTTPIngressPath{
			extbeta1.HTTPIngressPath{
				Path: fmt.Sprintf("/%s", serviceName),
				Backend: extbeta1.IngressBackend{
					ServiceName: serviceName,
					ServicePort: intstr.FromInt(80),
				},
			},
		},
	}
	ingress := extbeta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: ingressName,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class":                    ingressClass,
				"nginx.ingress.kubernetes.io/rewrite-target":     "/",
				"nginx.ingress.kubernetes.io/ssl-redirect":       "false",
				"nginx.ingress.kubernetes.io/proxy-body-size":    "100m",
				"nginx.ingress.kubernetes.io/proxy-send-timeout": "600",
				"nginx.ingress.kubernetes.io/proxy-read-timeout": "600",
			},
			Labels: makeIngressLabel(collectionID, projectID),
		},
		Spec: extbeta1.IngressSpec{
			Rules: []extbeta1.IngressRule{ingressRule},
		},
	}
	_, err := kcm.client.ExtensionsV1beta1().Ingresses(kcm.Namespace).Create(&ingress)
	if err != nil {
		log.Error(err)
	}
	return nil
}

func (kcm *K8sClientManager) deleteIngressRules(collectionID int64) error {
	deletePolicy := metav1.DeletePropagationForeground
	return kcm.client.ExtensionsV1beta1().Ingresses(kcm.Namespace).DeleteCollection(&metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("collection=%d", collectionID),
	})
}

func (kcm *K8sClientManager) CreateRoleBinding() error {
	namespace := kcm.Namespace
	nginxRoleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shibuya-ingress-role-binding",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      kcm.serviceAccount,
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "shibuya-ingress-role",
		},
	}
	_, err := kcm.client.RbacV1().RoleBindings(namespace).Create(nginxRoleBinding)
	if errors.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return nil
}

func (kcm *K8sClientManager) GetNodesByCollection(collectionID string) ([]apiv1.Node, error) {
	opts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("collection_id=%s", collectionID),
	}
	return kcm.getNodes(opts)
}

func (kcm *K8sClientManager) getNodes(opts metav1.ListOptions) ([]apiv1.Node, error) {
	nodeList, err := kcm.client.CoreV1().Nodes().List(opts)
	if err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

type NodesInfo struct {
	Size       int       `json:"size"`
	LaunchTime time.Time `json:"launch_time"`
}

type AllNodesInfo map[string]*NodesInfo

func (kcm *K8sClientManager) GetAllNodesInfo() (AllNodesInfo, error) {
	opts := metav1.ListOptions{}
	nodes, err := kcm.getNodes(opts)
	if err != nil {
		return nil, err
	}
	r := make(AllNodesInfo)
	for _, node := range nodes {
		nodeInfo := r[node.ObjectMeta.Labels["collection_id"]]
		if nodeInfo == nil {
			nodeInfo = &NodesInfo{}
			r[node.ObjectMeta.Labels["collection_id"]] = nodeInfo
		}
		nodeInfo.Size++
		if nodeInfo.LaunchTime.IsZero() || nodeInfo.LaunchTime.After(node.ObjectMeta.CreationTimestamp.Time) {
			nodeInfo.LaunchTime = node.ObjectMeta.CreationTimestamp.Time
		}
	}
	return r, nil
}

func (kcm *K8sClientManager) GetDeployedCollections() (map[int64]time.Time, error) {
	labelSelector := fmt.Sprintf("kind=executor")
	pods, err := kcm.GetPods(labelSelector, "")
	if err != nil {
		return nil, err
	}
	deployedCollections := make(map[int64]time.Time)
	for _, pod := range pods {
		collectionID, err := strconv.ParseInt(pod.Labels["collection"], 10, 64)
		if err != nil {
			return nil, err
		}
		deployedCollections[collectionID] = pod.CreationTimestamp.Time
	}
	return deployedCollections, nil
}

func (kcm *K8sClientManager) GetPodsMetrics(collectionID, planID int64) (map[string]apiv1.ResourceList, error) {
	metricsList, err := kcm.metricClient.MetricsV1beta1().PodMetricses(kcm.Namespace).List(metav1.ListOptions{
		LabelSelector: fmt.Sprintf("collection=%d,plan=%d", collectionID, planID),
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string]apiv1.ResourceList, len(metricsList.Items))
	for _, pm := range metricsList.Items {
		for _, cm := range pm.Containers {
			result[getEngineNumber(pm.GetName())] = cm.Usage
		}
	}
	return result, nil
}

func getEngineNumber(podName string) string {
	return strings.Split(podName, "-")[4]
}
