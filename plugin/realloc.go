package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	cpu       string
	mem       string
	bw        string
	namespace string
	pod       string
	container string
)

func init() {
	cpu = os.Getenv("KUBECTL_PLUGINS_LOCAL_FLAG_SET_CPU")
	mem = os.Getenv("KUBECTL_PLUGINS_LOCAL_FLAG_SET_MEMORY")
	bw = os.Getenv("KUBECTL_PLUGINS_LOCAL_FLAG_SET_BANDWIDTH")
	namespace = os.Getenv("KUBECTL_PLUGINS_LOCAL_FLAG_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}
	pod = os.Getenv("KUBECTL_PLUGINS_LOCAL_FLAG_POD")
	container = os.Getenv("KUBECTL_PLUGINS_LOCAL_FLAG_CONTAINER")
}

func main() {
	if pod == "" {
		fmt.Println("pod name is required")
		os.Exit(1)
	}

	if container == "" {
		fmt.Println("container name is required")
		os.Exit(1)
	}

	pc, err := getPodConfig(pod, namespace)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	containerID, err := pc.GetContainerID(container)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if cpu != "" {
		if err = post(pc.GetHost(), buildQuery(*containerID, CPU, cpu)); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	if mem != "" {
		if err = post(pc.GetHost(), buildQuery(*containerID, Memory, mem)); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	if bw != "" {
		if err = post(pc.GetHost(), buildQuery(*containerID, Bandwidth, bw)); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

type Resource string

const (
	CPU       Resource = "cpu"
	Memory    Resource = "memory"
	Bandwidth Resource = "bandwidth"
)

func buildQuery(containerID string, res Resource, value string) url.Values {
	v := url.Values{}

	v.Set("resource", string(res))
	v.Set("value", value)
	v.Set("container", containerID)

	return v
}

func post(host string, q url.Values) error {
	u, err := url.Parse(fmt.Sprintf("http://%s:8000", host))

	if err != nil {
		return err
	}

	u.RawQuery = q.Encode()

	resp, err := http.Post(u.String(), "application/json", nil)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	fmt.Println(string(buf))

	return nil
}

func getPodConfig(pod, ns string) (*PodConfiguration, error) {
	cmd := exec.Command("kubectl", []string{"get", "pod", pod, "-o=json"}...)

	out, err := cmd.Output()

	if err != nil {
		return nil, err
	}

	pc := &PodConfiguration{}

	if err = json.Unmarshal(out, pc); err != nil {
		return nil, err
	}

	return pc, nil
}

type PodConfiguration struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		CreationTimestamp time.Time `json:"creationTimestamp"`
		GenerateName      string    `json:"generateName"`
		Labels            struct {
			PodTemplateHash string `json:"pod-template-hash"`
			Run             string `json:"run"`
		} `json:"labels"`
		Name            string `json:"name"`
		Namespace       string `json:"namespace"`
		OwnerReferences []struct {
			APIVersion         string `json:"apiVersion"`
			BlockOwnerDeletion bool   `json:"blockOwnerDeletion"`
			Controller         bool   `json:"controller"`
			Kind               string `json:"kind"`
			Name               string `json:"name"`
			UID                string `json:"uid"`
		} `json:"ownerReferences"`
		ResourceVersion string `json:"resourceVersion"`
		SelfLink        string `json:"selfLink"`
		UID             string `json:"uid"`
	} `json:"metadata"`
	Spec struct {
		Containers []struct {
			Image           string `json:"image"`
			ImagePullPolicy string `json:"imagePullPolicy"`
			Name            string `json:"name"`
			Ports           []struct {
				ContainerPort int    `json:"containerPort"`
				Protocol      string `json:"protocol"`
			} `json:"ports"`
			Resources struct {
			} `json:"resources"`
			TerminationMessagePath   string `json:"terminationMessagePath"`
			TerminationMessagePolicy string `json:"terminationMessagePolicy"`
			VolumeMounts             []struct {
				MountPath string `json:"mountPath"`
				Name      string `json:"name"`
				ReadOnly  bool   `json:"readOnly"`
			} `json:"volumeMounts"`
		} `json:"containers"`
		DNSPolicy       string `json:"dnsPolicy"`
		NodeName        string `json:"nodeName"`
		RestartPolicy   string `json:"restartPolicy"`
		SchedulerName   string `json:"schedulerName"`
		SecurityContext struct {
		} `json:"securityContext"`
		ServiceAccount                string `json:"serviceAccount"`
		ServiceAccountName            string `json:"serviceAccountName"`
		TerminationGracePeriodSeconds int    `json:"terminationGracePeriodSeconds"`
		Tolerations                   []struct {
			Effect            string `json:"effect"`
			Key               string `json:"key"`
			Operator          string `json:"operator"`
			TolerationSeconds int    `json:"tolerationSeconds"`
		} `json:"tolerations"`
		Volumes []struct {
			Name   string `json:"name"`
			Secret struct {
				DefaultMode int    `json:"defaultMode"`
				SecretName  string `json:"secretName"`
			} `json:"secret"`
		} `json:"volumes"`
	} `json:"spec"`
	Status struct {
		Conditions []struct {
			LastProbeTime      interface{} `json:"lastProbeTime"`
			LastTransitionTime time.Time   `json:"lastTransitionTime"`
			Status             string      `json:"status"`
			Type               string      `json:"type"`
		} `json:"conditions"`
		ContainerStatuses []struct {
			ContainerID string `json:"containerID"`
			Image       string `json:"image"`
			ImageID     string `json:"imageID"`
			LastState   struct {
			} `json:"lastState"`
			Name         string `json:"name"`
			Ready        bool   `json:"ready"`
			RestartCount int    `json:"restartCount"`
			State        struct {
				Running struct {
					StartedAt time.Time `json:"startedAt"`
				} `json:"running"`
			} `json:"state"`
		} `json:"containerStatuses"`
		HostIP    string    `json:"hostIP"`
		Phase     string    `json:"phase"`
		PodIP     string    `json:"podIP"`
		QosClass  string    `json:"qosClass"`
		StartTime time.Time `json:"startTime"`
	} `json:"status"`
}

func (pc *PodConfiguration) GetContainerID(name string) (*string, error) {
	for _, container := range pc.Status.ContainerStatuses {
		if strings.EqualFold(container.Name, name) {
			id := strings.TrimLeft(container.ContainerID, "docker://")
			return &id, nil
		}
	}

	return nil, fmt.Errorf("container not found")
}

func (pc *PodConfiguration) GetHost() string {
	return pc.Status.HostIP
}
