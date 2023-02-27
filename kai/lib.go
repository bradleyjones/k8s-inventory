/*
Package kai retrieves Kubernetes In-Use Image data from the Kubernetes API. Runs adhoc and periodically, using the
k8s go SDK
*/package kai

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/anchore/kai/kai/presenter"
	"github.com/anchore/kai/kai/reporter"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/anchore/kai/internal/config"
	"github.com/anchore/kai/internal/log"
	"github.com/anchore/kai/internal/tracker"
	"github.com/anchore/kai/kai/client"
	"github.com/anchore/kai/kai/inventory"
	"github.com/anchore/kai/kai/logger"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ReportItem struct {
	Pods       []inventory.Pod
	Containers []inventory.Container
}

type channels struct {
	reportItem chan ReportItem
	errors     chan error
	stopper    chan struct{}
}

func HandleReport(report inventory.Report, cfg *config.Application) error {
	if cfg.AnchoreDetails.IsValid() {
		if err := reporter.Post(report, cfg.AnchoreDetails); err != nil {
			return fmt.Errorf("unable to report Inventory to Anchore: %w", err)
		}
		log.Info("Inventory report sent to Anchore")
	} else {
		log.Info("Anchore details not specified, not reporting inventory")
	}

	if cfg.VerboseInventoryReports {
		if err := presenter.GetPresenter(cfg.PresenterOpt, report).Present(os.Stdout); err != nil {
			return fmt.Errorf("unable to show inventory: %w", err)
		}
	}
	return nil
}

// PeriodicallyGetInventoryReport periodically retrieve image results and report/output them according to the configuration.
// Note: Errors do not cause the function to exit, since this is periodically running
func PeriodicallyGetInventoryReport(cfg *config.Application) {
	// Fire off a ticker that reports according to a configurable polling interval
	ticker := time.NewTicker(time.Duration(cfg.PollingIntervalSeconds) * time.Second)

	for {
		report, err := GetInventoryReport(cfg)
		if err != nil {
			log.Errorf("Failed to get Inventory Report: %w", err)
		} else {
			err := HandleReport(report, cfg)
			if err != nil {
				log.Errorf("Failed to handle Inventory Report: %w", err)
			}
		}

		log.Infof("Waiting %d seconds for next poll...", cfg.PollingIntervalSeconds)

		// Wait at least as long as the ticker
		log.Debugf("Start new gather: %s", <-ticker.C)
	}
}

// launchPodWorkerPool will create a worker pool of goroutines to grab pods
// from each namespace. This should alleviate the load on the api server
func launchPodWorkerPool(cfg *config.Application, kubeconfig *rest.Config, ch channels, queue chan inventory.Namespace) {
	for i := 0; i < cfg.Kubernetes.WorkerPoolSize; i++ {
		go func() {
			// each worker needs its own clientset
			clientset, err := client.GetClientSet(kubeconfig)
			if err != nil {
				ch.errors <- err
				return
			}

			for namespace := range queue {
				select {
				case <-ch.stopper:
					return
				default:
					fetchPodsInNamespace(clientset, cfg, namespace, ch)
				}
			}
		}()
	}
}

// TODO Find the actual count of images in use here?
func getImageCountFromResults(results []inventory.ReportItem) int {
	imageCount := 0
	for _, result := range results {
		imageCount += len(result.Images)
	}
	return imageCount
}

// GetInventoryReport is an atomic method for getting in-use image results, in parallel for multiple namespaces
func GetInventoryReport(cfg *config.Application) (inventory.Report, error) {
	log.Info("Starting image inventory collection")

	kubeconfig, err := client.GetKubeConfig(cfg)
	if err != nil {
		return inventory.Report{}, err
	}

	ch := channels{
		reportItem: make(chan ReportItem),
		errors:     make(chan error),
		stopper:    make(chan struct{}, 1),
	}

	namespaces, err := fetchNamespaces(kubeconfig, cfg)
	if err != nil {
		return inventory.Report{}, err
	}

	// fill the queue of namespaces to process
	queue := make(chan inventory.Namespace, len(namespaces))
	for _, n := range namespaces {
		queue <- n
	}
	close(queue)

	// get pods from namespaces using a worker pool pattern
	launchPodWorkerPool(cfg, kubeconfig, ch, queue)

	// listen for results from worker pool
	results := make([]ReportItem, 0)
	pods := make([]inventory.Pod, 0)
	containers := make([]inventory.Container, 0)
	for len(results) < len(namespaces) {
		select {
		case item := <-ch.reportItem:
			results = append(results, item)
			pods = append(pods, item.Pods...)
			containers = append(containers, item.Containers...)

		case err := <-ch.errors:
			close(ch.stopper)
			return inventory.Report{}, err

		case <-time.After(time.Second * time.Duration(cfg.Kubernetes.RequestTimeoutSeconds)):
			return inventory.Report{}, fmt.Errorf("timed out waiting for results")
		}
	}
	close(ch.reportItem)
	close(ch.errors)
	// safe to close here since the other channel close precedes a return statement
	close(ch.stopper)

	clientset, err := client.GetClientSet(kubeconfig)
	if err != nil {
		return inventory.Report{}, fmt.Errorf("failed to get k8s client set: %w", err)
	}

	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return inventory.Report{}, fmt.Errorf("failed to get Cluster Server Version: %w", err)
	}

	// TODO Re-enable once getImageCountFromResults is fixed
	// log.Infof(
	// 	"Got Inventory Report with %d images running across %d namespaces",
	// 	getImageCountFromResults(results),
	// 	len(results),
	// )
	log.Infof(
		"Got Inventory Report with %d containers running across %d namespaces",
		len(containers),
		len(results),
	)
	return inventory.Report{
		Timestamp:             time.Now().UTC().Format(time.RFC3339),
		Namepaces:             namespaces,
		Pods:                  pods,
		Containers:            containers,
		ServerVersionMetadata: serverVersion,
		ClusterName:           cfg.KubeConfig.Cluster,
		InventoryType:         "kubernetes",
	}, nil
}

// excludeCheck is a function that will return whether a namespace should be
// excluded based on a regex or direct string match
type excludeCheck func(namespace string) bool

// excludeRegex compiles a regex to use for namespace matching
func excludeRegex(check string) excludeCheck {
	return func(namespace string) bool {
		return regexp.MustCompile(check).MatchString(namespace)
	}
}

// excludeSet checks if a given string is present is a set
func excludeSet(check map[string]struct{}) excludeCheck {
	return func(namespace string) bool {
		_, exist := check[namespace]
		return exist
	}
}

// Regex to determine whether a string is a valid namespace (valid dns name)
var validNamespaceRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// buildExclusionChecklist will create a list of checks based on the configured
// exclusion strings. The checks could be regexes or direct string matches.
// It will create a regex check if the namespace string is not a valid dns
// name. If the namespace string in the exclude list is a valid dns name then
// it will add it to a map for direct lookup when the checks are run.
func buildExclusionChecklist(exclusions []string) []excludeCheck {
	var excludeChecks []excludeCheck

	if len(exclusions) > 0 {
		excludeMap := make(map[string]struct{})

		for _, ex := range exclusions {
			if !validNamespaceRegex.MatchString(ex) {
				// assume the check is a regex
				excludeChecks = append(excludeChecks, excludeRegex(ex))
			} else {
				// assume check is raw string so add to set for lookup
				excludeMap[ex] = struct{}{}
			}
		}
		excludeChecks = append(excludeChecks, excludeSet(excludeMap))
	}

	return excludeChecks
}

// excludeNamespace is a helper function to check whether a namespace matches
// any of the exclusion rules
func excludeNamespace(checks []excludeCheck, namespace string) bool {
	for _, check := range checks {
		if check(namespace) {
			return true
		}
	}
	return false
}

func fetchNamspace(clientset *kubernetes.Clientset, namespace string) (inventory.Namespace, error) {
	ns, err := clientset.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
	if err != nil {
		return inventory.Namespace{}, err
	}

	return inventory.Namespace{
		Annotations: ns.Annotations,
		Checksum:    "",
		Labels:      ns.Labels,
		Name:        ns.Name,
		Uid:         string(ns.UID),
	}, nil
}

// fetchNamespaces either return the namespaces detailed in the configuration
// OR if there are no namespaces listed in the configuration then it will
// return every namespace in the cluster.
func fetchNamespaces(kubeconfig *rest.Config, cfg *config.Application) ([]inventory.Namespace, error) {
	defer tracker.TrackFunctionTime(time.Now(), "Fetching namespaces")
	namespaces := make([]inventory.Namespace, 0)

	exclusionChecklist := buildExclusionChecklist(cfg.NamespaceSelectors.Exclude)

	// k8s clientset
	clientset, err := client.GetClientSet(kubeconfig)
	if err != nil {
		return []inventory.Namespace{}, fmt.Errorf("failed to get k8s client set: %w", err)
	}

	// Return list of namespaces if there are any present
	// First fetching the metadata for each namespace
	if len(cfg.NamespaceSelectors.Include) > 0 {
		namespaceNames := make([]string, 0)
		for _, ns := range cfg.NamespaceSelectors.Include {
			if !excludeNamespace(exclusionChecklist, ns) {
				namespaceNames = append(namespaceNames, ns)
				namespace, err := fetchNamspace(clientset, ns)
				if err != nil {
					return []inventory.Namespace{}, fmt.Errorf("failed to fetch namespace %s: %w", ns, err)
				}
				namespaces = append(namespaces, namespace)
			}
		}
		return namespaces, nil
	}

	// Otherwise collect all namespaces
	cont := ""
	for {
		opts := metav1.ListOptions{
			Limit:          cfg.Kubernetes.RequestBatchSize,
			Continue:       cont,
			TimeoutSeconds: &cfg.Kubernetes.RequestTimeoutSeconds,
		}

		list, err := clientset.CoreV1().Namespaces().List(context.TODO(), opts)
		if err != nil {
			// TODO: Handle HTTP 410 and recover
			return namespaces, fmt.Errorf("failed to list namespaces: %w", err)
		}

		for _, ns := range list.Items {
			if !excludeNamespace(exclusionChecklist, ns.ObjectMeta.Name) {
				namespace := inventory.Namespace{
					Annotations: ns.Annotations,
					Checksum:    "",
					Labels:      ns.Labels,
					Name:        ns.Name,
					Uid:         string(ns.UID),
				}
				namespaces = append(namespaces, namespace)
			}
		}

		cont = list.GetListMeta().GetContinue()

		if cont == "" {
			break
		}
	}
	return namespaces, nil
}

// Atomic Function that gets all the Namespace Images for a given searchNamespace and reports them to the unbuffered results channel
func fetchPodsInNamespace(clientset *kubernetes.Clientset, cfg *config.Application, ns inventory.Namespace, ch channels) {
	defer tracker.TrackFunctionTime(time.Now(), "Fetching pods in namespace: "+ns.Name)
	pods := make([]v1.Pod, 0)
	cont := ""
	for {
		opts := metav1.ListOptions{
			Limit:          cfg.Kubernetes.RequestBatchSize,
			Continue:       cont,
			TimeoutSeconds: &cfg.Kubernetes.RequestTimeoutSeconds,
		}

		list, err := clientset.CoreV1().Pods(ns.Name).List(context.TODO(), opts)
		if err != nil {
			// TODO: Handle HTTP 410 and recover
			ch.errors <- err
			return
		}

		pods = append(pods, list.Items...)

		cont = list.GetListMeta().GetContinue()

		if cont == "" {
			break
		}
	}

	log.Infof("There are %d pods in namespace \"%s\"", len(pods), ns)

	reportItem := ReportItem{
		Pods:       make([]inventory.Pod, 0),
		Containers: make([]inventory.Container, 0),
	}

	for _, pod := range pods {
		reportItem.Pods = append(reportItem.Pods, inventory.Pod{
			Annotations:  pod.Annotations,
			Checksum:     "",
			Labels:       pod.Labels,
			Name:         pod.Name,
			NamespaceUid: ns.Uid,
			NodeUid:      pod.Spec.NodeName, // TODO(bradjones) Should be Uid not Name
			Uid:          string(pod.UID),
		})
		reportItem.Containers = append(reportItem.Containers, getContainersFromPod(pod)...)
	}
	// ch.reportItem <- inventory.NewReportItem(pods, ns, cfg.IgnoreNotRunning, cfg.MissingTagPolicy.Policy, cfg.MissingTagPolicy.Tag)
	ch.reportItem <- reportItem
}

func getContainersFromPod(pod v1.Pod) []inventory.Container {
	// Look at both status for init and regular containers
	// Must use status when looking at containers in order to obtain the container ID
	// TODO(bradjones) - may consider also looking at the spec for more details
	containers := make([]inventory.Container, 0)

	for _, c := range pod.Status.InitContainerStatuses {
		containers = append(containers, inventory.Container{
			Id:          c.ContainerID,
			PodUid:      string(pod.UID),
			ImageTag:    c.Image,
			ImageDigest: c.ImageID,
			Name:        c.Name,
		})
	}
	for _, c := range pod.Status.ContainerStatuses {
		containers = append(containers, inventory.Container{
			Id:          c.ContainerID,
			PodUid:      string(pod.UID),
			ImageTag:    c.Image,
			ImageDigest: c.ImageID,
			Name:        c.Name,
		})
	}
	return containers
}

func SetLogger(logger logger.Logger) {
	log.Log = logger
}
