package inventory

import (
	"k8s.io/apimachinery/pkg/version"
)

type Namespace struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
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
	Annotations             map[string]string `json:"annotations,omitempty"`
	Arch                    string            `json:"arch,omitempty"`
	ContainerRuntimeVersion string            `json:"container_runtime_version,omitempty"`
	KernelVersion           string            `json:"kernel_version,omitempty"`
	KubeProxyVersion        string            `json:"kube_proxy_version,omitempty"`
	KubeletVersion          string            `json:"kubelet_version,omitempty"`
	KubernetesVersion       string            `json:"kubernetes_version,omitempty"`
	Labels                  map[string]string `json:"labels,omitempty"`
	Name                    string            `json:"name"`
	OperatingSystem         string            `json:"operating_system,omitempty"`
	Uid                     string            `json:"uid"`
}

type Pod struct {
	Annotations  map[string]string `json:"annotations,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Name         string            `json:"name"`
	NamespaceUid string            `json:"namespace_uid"`
	NodeUid      string            `json:"node_uid"`
	Uid          string            `json:"uid"`
}

type Report struct {
	ClusterName           string        `json:"cluster_name"`
	Containers            []Container   `json:"containers"`
	Namepaces             []Namespace   `json:"namespaces,omitempty"`
	Nodes                 []Node        `json:"nodes,omitempty"`
	Pods                  []Pod         `json:"pods,omitempty"`
	ServerVersionMetadata *version.Info `json:"serverVersionMetadata"`
	Timestamp             string        `json:"timestamp,omitempty"` // Should be generated using time.Now.UTC() and formatted according to RFC Y-M-DTH:M:SZ
}
