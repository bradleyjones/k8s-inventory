package inventory

import (
	"k8s.io/apimachinery/pkg/version"
)

type Namespace struct {
	Annotations map[string]string `json:"annotations"`
	Checksum    string            `json:"checksum"`
	Labels      map[string]string `json:"labels"`
	Name        string            `json:"name"`
	Uid         string            `json:"uid"`
}

// TODO (bradjones) Add container state (running, terminating, etc.)
type Container struct {
	Id          string `json:"id"`
	ImageDigest string `json:"image_digest"`
	ImageTag    string `json:"image_tag"`
	Name        string `json:"name"`
	PodUid      string `json:"pod_uid"`
}

type Node struct {
	Annotations             map[string]string `json:"annotations"`
	Arch                    string            `json:"arch"`
	Checksum                string            `json:"checksum"`
	ContainerRuntimeVersion string            `json:"container_runtime_version"`
	KernelVersion           string            `json:"kernel_version"`
	KubeProxyVersion        string            `json:"kube_proxy_version"`
	KubeletVersion          string            `json:"kubelet_version"`
	KubernetesVersion       string            `json:"kubernetes_version"`
	Labels                  map[string]string `json:"labels"`
	Name                    string            `json:"name"`
	OperatingSystem         string            `json:"operating_system"`
	Uid                     string            `json:"uid"`
}

type Pod struct {
	Annotations  map[string]string `json:"annotations"`
	Checksum     string            `json:"checksum"`
	Labels       map[string]string `json:"labels"`
	Name         string            `json:"name"`
	NamespaceUid string            `json:"namespace_uid"`
	NodeUid      string            `json:"node_uid"`
	Uid          string            `json:"uid"`
}

type Report struct {
	ClusterName           string        `json:"cluster_name"`
	Containers            []Container   `json:"containers"`
	InventoryType         string        `json:"inventory_type"`
	Namepaces             []Namespace   `json:"namespaces"`
	Nodes                 []Node        `json:"nodes"`
	Pods                  []Pod         `json:"pods"`
	ServerVersionMetadata *version.Info `json:"serverVersionMetadata"`
	Timestamp             string        `json:"timestamp,omitempty"` // Should be generated using time.Now.UTC() and formatted according to RFC Y-M-DTH:M:SZ
}
