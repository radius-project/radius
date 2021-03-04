package kubernetes_helpers

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// ValidatePodsRunning validates the namespaces and pods specified in each namespace are running
func ValidatePodsRunning(expectedPods map[string]int) bool {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	for namespace, expectedNumPods := range expectedPods {
		pods, _ := clientset.CoreV1().Pods(namespace).List(context.TODO(), v1.ListOptions{})
		if len(pods.Items) != expectedNumPods {
			fmt.Printf("Number of pods: %v found inside namespace: %v does not match the expected value: %v", len(pods.Items), namespace, expectedNumPods)
			return false
		}
		for _, p := range pods.Items {
			if p.Status.Phase != "Running" {
				fmt.Printf("Pod: %v is not in Running state", p.Name)
				return false
			}
		}
	}
	return true
}
