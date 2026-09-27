package installer

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func WaitForReady(ctx context.Context, releaseName, namespace string, timeout time.Duration, matchLabels map[string]string) error {
	config, err := getKubeConfig()
	if err != nil {
		return fmt.Errorf("kube config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("kubernetes client: %w", err)
	}
	if namespace == "" {
		namespace = "default"
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %q to be ready", releaseName)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ready, err := checkPodsReady(ctx, clientset, releaseName, namespace, matchLabels)
			if err != nil {
				return err
			}
			if ready {
				return nil
			}
		}
	}
}

func getKubeConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	return kubeconfig.ClientConfig()
}

func checkPodsReady(ctx context.Context, clientset *kubernetes.Clientset, releaseName, namespace string, matchLabels map[string]string) (bool, error) {
	var selector string
	if len(matchLabels) > 0 {
		selector = labels.SelectorFromSet(matchLabels).String()
	} else {
		selector = "app.kubernetes.io/instance=" + releaseName
	}
	list, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return false, fmt.Errorf("listing pods: %w", err)
	}
	return podsReady(list.Items), nil
}

// podsReady reports whether every selected pod is done starting: running with
// all containers ready, or -- for the pod of a Job -- completed successfully.
// A Succeeded pod is never Ready (its containers have exited), so without the
// second case a requirement on a Job could only ever time out.
func podsReady(pods []corev1.Pod) bool {
	if len(pods) == 0 {
		return false
	}
	for _, pod := range pods {
		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			continue
		case corev1.PodRunning:
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}
