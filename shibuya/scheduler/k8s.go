package scheduler

import (
	"context"
	e "errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rakutentech/shibuya/shibuya/config"
	"github.com/rakutentech/shibuya/shibuya/engines/metrics"
	model "github.com/rakutentech/shibuya/shibuya/model"
	"github.com/rakutentech/shibuya/shibuya/object_storage"
	smodel "github.com/rakutentech/shibuya/shibuya/scheduler/model"
	log "github.com/sirupsen/logrus"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	v1networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

type K8sClientManager struct {
	*config.ExecutorConfig
	client         *kubernetes.Clientset
	serviceAccount string
}

func NewK8sClientManager(cfg *config.ClusterConfig) *K8sClientManager {
	c, err := config.GetKubeClient()
	if err != nil {
		log.Warning(err)
	}
	return &K8sClientManager{
		config.SC.ExecutorConfig, c, "shibuya-ingress-serviceaccount-1",
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

func prepareEngineMetaEnvvars(collectionID, planID int64) []apiv1.EnvVar {
	return []apiv1.EnvVar{
		{
			Name:  "collection_id",
			Value: fmt.Sprintf("%d", collectionID),
		},
		{
			Name:  "plan_id",
			Value: fmt.Sprintf("%d", planID),
		},
	}
}

func makeTolerations(key string, value string, effect apiv1.TaintEffect) apiv1.Toleration {
	toleration := apiv1.Toleration{
		Effect:   effect,
		Key:      key,
		Operator: apiv1.TolerationOpEqual,
		Value:    value,
	}
	return toleration
}

func collectionNodeAffinity(collectionID int64) *apiv1.NodeAffinity {
	collectionIDStr := fmt.Sprintf("%d", collectionID)
	return makeNodeAffinity("collection_id", collectionIDStr)
}

func makePodAffinity(key, value string) *apiv1.PodAffinity {
	podAffinity := &apiv1.PodAffinity{
		PreferredDuringSchedulingIgnoredDuringExecution: []apiv1.WeightedPodAffinityTerm{
			{
				Weight: 100,
				PodAffinityTerm: apiv1.PodAffinityTerm{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							key: value,
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}
	return podAffinity
}

func collectionPodAffinity(collectionID int64) *apiv1.PodAffinity {
	collectionIDStr := fmt.Sprintf("%d", collectionID)
	return makePodAffinity("collection", collectionIDStr)
}

func prepareAffinity(collectionID int64) *apiv1.Affinity {
	affinity := &apiv1.Affinity{}
	affinity.PodAffinity = collectionPodAffinity(collectionID)
	na := config.SC.ExecutorConfig.NodeAffinity
	if len(na) > 0 {
		t := na[0]
		affinity.NodeAffinity = makeNodeAffinity(t["key"], t["value"])
		return affinity
	}
	return affinity
}

func prepareTolerations() []apiv1.Toleration {
	tolerations := []apiv1.Toleration{}
	na := config.SC.ExecutorConfig.Tolerations

	if len(na) > 0 {
		for _, t := range na {
			tolerations = append(tolerations, makeTolerations(t.Key, t.Value, t.Effect))
		}
	}
	return tolerations
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

func (kcm *K8sClientManager) generatePlanDeployment(planName string, replicas int, labels map[string]string, containerConfig *config.ExecutorContainer,
	affinity *apiv1.Affinity, tolerations []apiv1.Toleration, envvars []apiv1.EnvVar) appsv1.StatefulSet {
	t := true
	volumes := []apiv1.Volume{}
	volumeMounts := []apiv1.VolumeMount{}
	if object_storage.IsProviderGCP() {
		volumeName := "shibuya-gcp-auth"
		secretName := config.SC.ObjectStorage.SecretName
		authFileName := config.SC.ObjectStorage.AuthFileName
		mountPath := fmt.Sprintf("/auth/%s", authFileName)
		v := apiv1.Volume{
			Name: volumeName,
			VolumeSource: apiv1.VolumeSource{
				Secret: &apiv1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		}
		volumes = append(volumes, v)
		vm := apiv1.VolumeMount{
			Name:      volumeName,
			MountPath: mountPath,
			SubPath:   authFileName,
		}
		volumeMounts = append(volumeMounts, vm)
		envvar := apiv1.EnvVar{
			Name:  "GOOGLE_APPLICATION_CREDENTIALS",
			Value: mountPath,
		}
		envvars = append(envvars, envvar)
	}
	cmVolumeName := "shibuya-config"
	cmName := config.SC.ObjectStorage.ConfigMapName
	cmVolume := apiv1.Volume{
		Name: cmVolumeName,
		VolumeSource: apiv1.VolumeSource{
			ConfigMap: &apiv1.ConfigMapVolumeSource{
				LocalObjectReference: apiv1.LocalObjectReference{
					Name: cmName,
				},
			},
		},
	}
	volumes = append(volumes, cmVolume)
	cmVolumeMounts := apiv1.VolumeMount{
		Name:      cmVolumeName,
		MountPath: config.ConfigFilePath,
		SubPath:   config.ConfigFileName,
	}
	volumeMounts = append(volumeMounts, cmVolumeMounts)
	deployment := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:                       planName,
			DeletionGracePeriodSeconds: new(int64),
			Labels:                     labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:            int32Ptr(int32(replicas)),
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: apiv1.PodSpec{
					Affinity:                     affinity,
					Tolerations:                  tolerations,
					ServiceAccountName:           kcm.serviceAccount,
					AutomountServiceAccountToken: &t,
					ImagePullSecrets: []apiv1.LocalObjectReference{
						{
							Name: kcm.ImagePullSecret,
						},
					},
					TerminationGracePeriodSeconds: new(int64),
					HostAliases:                   kcm.makeHostAliases(),
					Volumes:                       volumes,
					Containers: []apiv1.Container{
						{
							Name:            planName,
							Image:           containerConfig.Image,
							ImagePullPolicy: kcm.ImagePullPolicy,
							Env:             envvars,
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
							VolumeMounts: volumeMounts,
						},
					},
				},
			},
		},
	}
	return deployment
}

func (kcm *K8sClientManager) generateEngineDeployment(engineName string, labels map[string]string,
	containerConfig *config.ExecutorContainer, affinity *apiv1.Affinity,
	tolerations []apiv1.Toleration) appsv1.Deployment {
	t := true
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:                       engineName,
			DeletionGracePeriodSeconds: new(int64),
			Labels:                     labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: apiv1.PodSpec{
					Affinity:                     affinity,
					Tolerations:                  tolerations,
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
							Name:            engineName,
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
	_, err := deploymentsClient.Create(context.TODO(), deployment, metav1.CreateOptions{})
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
		switch kcm.Cluster.ServiceType {
		case "NodePort":
			service.Spec.Type = apiv1.ServiceTypeNodePort
		case "LoadBalancer":
			service.Spec.ExternalTrafficPolicy = "Local"
			service.Spec.Type = apiv1.ServiceTypeLoadBalancer
		}
	}
	_, err := kcm.client.CoreV1().Services(kcm.Namespace).Create(context.TODO(), service, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (kcm *K8sClientManager) getRandomHostIP() (string, error) {
	podList, err := kcm.client.CoreV1().Pods(kcm.Namespace).
		List(context.TODO(), metav1.ListOptions{
			Limit: 1,
			// we need to add the selector here because pod's hostIP could be empty if it's in pending state
			// So we want to only find the pod that is running so it would have hostIP.
			FieldSelector: fmt.Sprintf("status.phase=Running"),
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

func (kcm *K8sClientManager) DeployEngine(projectID, collectionID, planID int64,
	engineID int, containerConfig *config.ExecutorContainer) error {
	engineName := makeEngineName(projectID, collectionID, planID, engineID)
	labels := makeEngineLabel(projectID, collectionID, planID, engineName)
	affinity := prepareAffinity(collectionID)
	tolerations := prepareTolerations()
	engineConfig := kcm.generateEngineDeployment(engineName, labels, containerConfig, affinity, tolerations)
	if err := kcm.deploy(&engineConfig); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	engineSvcName := makeEngineName(projectID, collectionID, planID, engineID)
	if err := kcm.CreateService(engineSvcName, engineConfig); err != nil {
		return err
	}
	ingressClass := makeIngressClass(projectID)
	ingressName := makeIngressName(projectID, collectionID, planID, engineID)
	if err := kcm.CreateIngress(ingressClass, ingressName, engineSvcName, collectionID, projectID); err != nil {
		return err
	}
	log.Printf("Finish creating one engine for %s", engineName)
	return nil
}

func (kcm *K8sClientManager) makePlanService(name string, label map[string]string) *apiv1.Service {
	service := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				"networking.istio.io/exportTo": ".",
			},
			Labels: label,
		},
		Spec: apiv1.ServiceSpec{
			Type:      apiv1.ServiceTypeClusterIP,
			ClusterIP: "None",
			Selector:  label,
			Ports: []apiv1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
			},
		},
	}
	return service
}

func (kcm *K8sClientManager) DeployPlan(projectID, collectionID, planID int64, enginesNo int, containerconfig *config.ExecutorContainer) error {
	planName := makePlanName(projectID, collectionID, planID)
	labels := makePlanLabel(projectID, collectionID, planID)
	affinity := prepareAffinity(collectionID)
	envvars := prepareEngineMetaEnvvars(collectionID, planID)
	tolerations := prepareTolerations()
	planConfig := kcm.generatePlanDeployment(planName, enginesNo, labels, containerconfig, affinity, tolerations, envvars)
	if _, err := kcm.client.AppsV1().StatefulSets(kcm.Namespace).Create(context.TODO(), &planConfig, metav1.CreateOptions{}); err != nil {
		return err
	}
	service := kcm.makePlanService(planName, labels)
	if _, err := kcm.client.CoreV1().Services(kcm.Namespace).Create(context.TODO(), service, metav1.CreateOptions{}); err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func (kcm *K8sClientManager) GetIngressUrl(projectID int64) (string, error) {
	igName := makeIngressClass(projectID)
	serviceClient, err := kcm.client.CoreV1().Services(kcm.Namespace).
		Get(context.TODO(), igName, metav1.GetOptions{})
	if err != nil {
		return "", makeSchedulerIngressError(err)
	}
	if kcm.InCluster {
		return serviceClient.Spec.ClusterIP, nil
	}
	if kcm.Cluster.ServiceType == "LoadBalancer" {
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
		List(context.TODO(), metav1.ListOptions{
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

func (kcm *K8sClientManager) GetEnginesByProject(projectID int64) ([]apiv1.Pod, error) {
	labelSelector := fmt.Sprintf("project=%d, kind=executor", projectID)
	pods, err := kcm.GetPods(labelSelector, "")
	if err != nil {
		return nil, err
	}
	sort.Slice(pods, func(i, j int) bool {
		p1 := pods[i]
		p2 := pods[j]
		return p1.CreationTimestamp.Time.After(p2.CreationTimestamp.Time)
	})
	return pods, nil
}

func (kcm *K8sClientManager) FetchEngineUrlsByPlan(collectionID, planID int64, opts *smodel.EngineOwnerRef) ([]string, error) {
	collectionUrl, err := kcm.GetIngressUrl(opts.ProjectID)
	if err != nil {
		return nil, err
	}
	urls := []string{}
	for i := 0; i < opts.EnginesCount; i++ {
		engineSvcName := makeEngineName(opts.ProjectID, collectionID, planID, i)
		u := fmt.Sprintf("%s/%s", collectionUrl, engineSvcName)
		urls = append(urls, u)
	}
	return urls, nil
}

func (kcm *K8sClientManager) CollectionStatus(projectID, collectionID int64, eps []*model.ExecutionPlan) (*smodel.CollectionStatus, error) {
	planStatuses := make(map[int64]*smodel.PlanStatus)
	var engineReachable bool
	cs := &smodel.CollectionStatus{}
	pods := kcm.GetPodsByCollection(collectionID, "")
	ingressControllerDeployed := false
	for _, ep := range eps {
		ps := &smodel.PlanStatus{
			PlanID:  ep.PlanID,
			Engines: ep.Engines,
		}
		planStatuses[ep.PlanID] = ps
	}
	enginesReady := true
	scraperDeployed := false
	for _, pod := range pods {
		if pod.Labels["kind"] == "ingress-controller" {
			ingressControllerDeployed = true
			continue
		}
		if pod.Labels["kind"] == "scraper" {
			if pod.Status.Phase == apiv1.PodRunning {
				scraperDeployed = true
				continue
			}
			continue
		}
		planID, err := strconv.Atoi(pod.Labels["plan"])
		if err != nil {
			log.Error(err)
		}
		ps, ok := planStatuses[int64(planID)]
		if !ok {
			log.Error("Could not find running pod in ExecutionPlan")
			continue
		}
		ps.EnginesDeployed += 1
		if pod.Status.Phase != apiv1.PodRunning {
			enginesReady = false
		}
	}
	// if it's unrechable, we can assume it's not in progress as well
	fieldSelector := fmt.Sprintf("status.phase=Running")
	ingressPods := kcm.GetPodsByCollection(collectionID, fieldSelector)
	ingressControllerDeployed = len(ingressPods) >= 1
	if !ingressControllerDeployed || !enginesReady || !scraperDeployed {
		for _, ps := range planStatuses {
			cs.Plans = append(cs.Plans, ps)
		}
		return cs, nil
	}
	engineReachable = false
	randomPlan := eps[0]
	opts := &smodel.EngineOwnerRef{
		ProjectID:    projectID,
		EnginesCount: randomPlan.Engines,
	}
	engineUrls, err := kcm.FetchEngineUrlsByPlan(collectionID, randomPlan.PlanID, opts)
	if err == nil {
		randomEngine := engineUrls[0]
		engineReachable = kcm.ServiceReachable(randomEngine)
	}
	jobs := make(chan *smodel.PlanStatus)
	result := make(chan *smodel.PlanStatus)
	for w := 0; w < len(eps); w++ {
		go smodel.GetPlanStatus(collectionID, jobs, result)
	}
	for _, ps := range planStatuses {
		jobs <- ps
	}
	defer close(jobs)
	defer close(result)
	for range eps {
		ps := <-result
		if ps.Engines == ps.EnginesDeployed && engineReachable {
			ps.EnginesReachable = true
		}
		cs.Plans = append(cs.Plans, ps)
	}
	return cs, nil
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
	readCloser, err := req.Stream(context.TODO())
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

func (kcm *K8sClientManager) PodReadyCount(collectionID int64) int {
	label := makeCollectionLabel(collectionID)
	podsClient, err := kcm.client.CoreV1().Pods(kcm.Namespace).
		List(context.TODO(), metav1.ListOptions{
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

func (kcm *K8sClientManager) ServiceReachable(engineUrl string) bool {
	resp, err := http.Get(fmt.Sprintf("http://%s/start", engineUrl))
	if err != nil {
		log.Warn(err)
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (kcm *K8sClientManager) deleteService(collectionID int64) error {
	// We could not delete services by label
	// So we firstly get them by label and then delete them one by one
	// you can check here: https://github.com/kubernetes/kubernetes/issues/68468#issuecomment-419981870
	corev1Client := kcm.client.CoreV1().Services(kcm.Namespace)
	resp, err := corev1Client.List(context.TODO(), metav1.ListOptions{
		LabelSelector: makeCollectionLabel(collectionID),
	})
	if err != nil {
		return err
	}

	// If there are any errors in deletion, we only return the last one
	// the errors could be similar so we should avoid return a long list of errors
	var lastError error
	for _, svc := range resp.Items {
		if err := corev1Client.Delete(context.TODO(), svc.Name, metav1.DeleteOptions{}); err != nil {
			lastError = err
		}
	}
	return lastError
}

func (kcm *K8sClientManager) deleteDeployment(collectionID int64) error {
	ls := fmt.Sprintf("collection=%d", collectionID)
	deploymentsClient := kcm.client.AppsV1().Deployments(kcm.Namespace)
	err := deploymentsClient.DeleteCollection(context.TODO(), metav1.DeleteOptions{
		GracePeriodSeconds: new(int64),
	}, metav1.ListOptions{
		LabelSelector: ls,
	})
	if err != nil {
		log.Error(err)
		return err
	}
	if err := kcm.client.AppsV1().StatefulSets(kcm.Namespace).DeleteCollection(context.TODO(),
		metav1.DeleteOptions{GracePeriodSeconds: new(int64)}, metav1.ListOptions{LabelSelector: ls}); err != nil {
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
	if err := kcm.client.CoreV1().ConfigMaps(kcm.Namespace).Delete(context.TODO(),
		makePromConfigName(collectionID), metav1.DeleteOptions{}); err != nil {
		return err
	}
	if err := kcm.client.AppsV1().StatefulSets(kcm.Namespace).Delete(context.TODO(), makeScraperDeploymentName(collectionID),
		metav1.DeleteOptions{}); err != nil {
		return err
	}
	return nil
}

func (kcm *K8sClientManager) PurgeProjectIngress(projectID int64) error {
	igName := makeIngressClass(projectID)
	deleteOpts := metav1.DeleteOptions{}
	if err := kcm.client.AppsV1().Deployments(kcm.Namespace).Delete(context.TODO(), igName, deleteOpts); err != nil {
		return err
	}
	if err := kcm.client.CoreV1().Services(kcm.Namespace).Delete(context.TODO(), igName, deleteOpts); err != nil {
		return err
	}
	return nil
}

func int32Ptr(i int32) *int32 { return &i }

func (kcm *K8sClientManager) generateControllerDeployment(igName string, projectID int64) appsv1.Deployment {
	tolerations := prepareTolerations()
	t := true
	labels := makeIngressControllerLabel(projectID)
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   igName,
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(config.SC.IngressConfig.Replicas),
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
					Tolerations:                   tolerations,
					ServiceAccountName:            kcm.serviceAccount,
					TerminationGracePeriodSeconds: new(int64),
					AutomountServiceAccountToken:  &t,
					Containers: []apiv1.Container{
						{
							Name:  "shibuya-ingress-controller",
							Image: config.SC.IngressConfig.Image,
							Resources: apiv1.ResourceRequirements{
								// Limits are whatever Kubernetes sets as the max value
								Requests: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse(config.SC.IngressConfig.CPU),
									apiv1.ResourceMemory: resource.MustParse(config.SC.IngressConfig.Mem),
								},
								Limits: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse(config.SC.IngressConfig.CPU),
									apiv1.ResourceMemory: resource.MustParse(config.SC.IngressConfig.Mem),
								},
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
									ContainerPort: 8080,
								},
								{
									Name:          "https",
									ContainerPort: 443,
								},
							},
							Env: []apiv1.EnvVar{
								{
									Name: "POD_NAME",
									ValueFrom: &apiv1.EnvVarSource{
										FieldRef: &apiv1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "POD_NAMESPACE",
									ValueFrom: &apiv1.EnvVarSource{
										FieldRef: &apiv1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name:  "project_id",
									Value: fmt.Sprintf("%d", projectID),
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

func (kcm *K8sClientManager) makeScraperConfig(collectionID int64) (apiv1.ConfigMap, error) {
	empty := apiv1.ConfigMap{}
	pc, err := metrics.MakeScraperConfig(collectionID, kcm.Namespace, config.SC.MetricStorage)
	if err != nil {
		return empty, err
	}
	c, err := yaml.Marshal(pc)
	if err != nil {
		return empty, err
	}
	data := map[string]string{}
	data["prometheus.yml"] = string(c)
	labels := makeScraperLabel(collectionID)
	return apiv1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makePromConfigName(collectionID),
			Namespace: kcm.Namespace,
			Labels:    labels,
		},
		Data: data,
	}, nil
}

func (kcm *K8sClientManager) makeScraperDeployment(collectionID int64) appsv1.StatefulSet {
	workloadName := makeScraperDeploymentName(collectionID)
	labels := makeScraperLabel(collectionID)
	// Currently scraper shares the affinity and tolerations with executors
	affinity := prepareAffinity(collectionID)
	tolerations := prepareTolerations()
	scraperContainer := config.SC.ScraperContainer
	return appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadName,
			Namespace: kcm.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: apiv1.PodSpec{
					ServiceAccountName: kcm.serviceAccount,
					Affinity:           affinity,
					Tolerations:        tolerations,
					Containers: []apiv1.Container{
						{
							Name:  "prom",
							Image: scraperContainer.Image,
							Resources: apiv1.ResourceRequirements{
								Limits: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse(scraperContainer.CPU),
									apiv1.ResourceMemory: resource.MustParse(scraperContainer.Mem),
								},
								Requests: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse(scraperContainer.CPU),
									apiv1.ResourceMemory: resource.MustParse(scraperContainer.Mem),
								},
							},
							Ports: []apiv1.ContainerPort{
								{
									ContainerPort: int32(9090),
								},
							},
							VolumeMounts: []apiv1.VolumeMount{
								{
									Name:      "prom-config",
									MountPath: "/etc/prometheus",
								},
							},
						},
					},
					Volumes: []apiv1.Volume{
						{
							Name: "prom-config",
							VolumeSource: apiv1.VolumeSource{
								ConfigMap: &apiv1.ConfigMapVolumeSource{
									LocalObjectReference: apiv1.LocalObjectReference{
										Name: makePromConfigName(collectionID),
									},
									DefaultMode: int32Ptr(420),
								},
							},
						},
					},
				},
			},
		},
	}
}

func (kcm *K8sClientManager) CreateCollectionScraper(collectionID int64) error {
	promDeployment := kcm.makeScraperDeployment(collectionID)
	promConfig, err := kcm.makeScraperConfig(collectionID)
	if err != nil {
		return err
	}
	if _, err := kcm.client.CoreV1().ConfigMaps(kcm.Namespace).Create(context.TODO(), &promConfig, metav1.CreateOptions{}); err != nil {
		if errors.IsAlreadyExists(err) {
			return nil
		}
	}
	if _, err := kcm.client.AppsV1().StatefulSets(kcm.Namespace).Create(context.TODO(), &promDeployment, metav1.CreateOptions{}); err != nil {
		if errors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil
}

func (kcm *K8sClientManager) ExposeProject(projectID int64) error {
	igName := makeIngressClass(projectID)
	deployment := kcm.generateControllerDeployment(igName, projectID)
	// there could be duplicated controller deployment from multiple collections
	// This method has already taken it into considertion.
	if err := kcm.deploy(&deployment); err != nil {
		return err
	}
	if err := kcm.expose(igName, &deployment); err != nil {
		return err
	}
	return nil
}

func (kcm *K8sClientManager) CreateIngress(ingressClass, ingressName, serviceName string, collectionID, projectID int64) error {
	ingressRule := v1networking.IngressRule{}
	pathType := v1networking.PathType("Exact")
	ingressRule.HTTP = &v1networking.HTTPIngressRuleValue{
		Paths: []v1networking.HTTPIngressPath{
			{
				Path:     fmt.Sprintf("/%s/(.*)", serviceName),
				PathType: &pathType,
				Backend: v1networking.IngressBackend{
					Service: &v1networking.IngressServiceBackend{
						Name: serviceName,
						Port: v1networking.ServiceBackendPort{
							Number: 80,
						},
					},
				},
			},
		},
	}
	ingress := v1networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: ingressName,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class":                    ingressClass,
				"nginx.ingress.kubernetes.io/rewrite-target":     "/$1",
				"nginx.ingress.kubernetes.io/ssl-redirect":       "false",
				"nginx.ingress.kubernetes.io/proxy-body-size":    "100m",
				"nginx.ingress.kubernetes.io/proxy-send-timeout": "600",
				"nginx.ingress.kubernetes.io/proxy-read-timeout": "600",
			},
			Labels: makeIngressLabel(projectID, collectionID),
		},
		Spec: v1networking.IngressSpec{
			Rules: []v1networking.IngressRule{ingressRule},
		},
	}
	_, err := kcm.client.NetworkingV1().Ingresses(kcm.Namespace).Create(context.TODO(), &ingress, metav1.CreateOptions{})
	if err != nil {
		log.Error(err)
	}
	return nil
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

func (kcm *K8sClientManager) GetDeployedServices() (map[int64]time.Time, error) {
	labelSelector := fmt.Sprintf("kind=ingress-controller")
	services, err := kcm.client.CoreV1().Services(kcm.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	deployedServices := make(map[int64]time.Time)
	for _, svc := range services.Items {
		projectID, err := strconv.ParseInt(svc.Labels["project"], 10, 64)
		if err != nil {
			return nil, err
		}
		deployedServices[projectID] = svc.CreationTimestamp.Time
	}
	return deployedServices, nil
}

func (kcm *K8sClientManager) GetCollectionEnginesDetail(projectID, collectionID int64) (*smodel.CollectionDetails, error) {
	labelSelector := fmt.Sprintf("collection=%d", collectionID)
	pods, err := kcm.GetPods(labelSelector, "")
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		return nil, &NoResourcesFoundErr{Err: err, Message: "Cannot find the engines"}
	}
	collectionDetails := new(smodel.CollectionDetails)
	ingressUrl, err := kcm.GetIngressUrl(projectID)
	if err != nil {
		collectionDetails.IngressIP = err.Error()
	} else {
		collectionDetails.IngressIP = ingressUrl
	}
	engines := []*smodel.EngineStatus{}
	for _, p := range pods {
		es := new(smodel.EngineStatus)
		if kind, _ := p.Labels["kind"]; kind != "executor" {
			continue
		}
		es.Name = p.Name
		es.CreatedTime = p.ObjectMeta.CreationTimestamp.Time
		es.Status = string(p.Status.Phase)
		engines = append(engines, es)
	}
	collectionDetails.Engines = engines
	collectionDetails.ControllerReplicas = config.SC.IngressConfig.Replicas
	return collectionDetails, nil
}

func getEngineNumber(podName string) string {
	return strings.Split(podName, "-")[4]
}
