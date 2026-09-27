package installer

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func pod(phase corev1.PodPhase, ready ...bool) corev1.Pod {
	p := corev1.Pod{Status: corev1.PodStatus{Phase: phase}}
	for _, r := range ready {
		p.Status.ContainerStatuses = append(p.Status.ContainerStatuses, corev1.ContainerStatus{Ready: r})
	}
	return p
}

func TestPodsReady(t *testing.T) {
	cases := []struct {
		name string
		pods []corev1.Pod
		want bool
	}{
		{"no pods yet", nil, false},
		{"running and ready", []corev1.Pod{pod(corev1.PodRunning, true, true)}, true},
		{"running, one container not ready", []corev1.Pod{pod(corev1.PodRunning, true, false)}, false},
		{"pending", []corev1.Pod{pod(corev1.PodPending)}, false},
		// A Job's pod ends Succeeded, never Ready: that is what completion looks like.
		{"job pod succeeded", []corev1.Pod{pod(corev1.PodSucceeded, false)}, true},
		{"job pod failed", []corev1.Pod{pod(corev1.PodFailed, false)}, false},
		{"succeeded and running ready", []corev1.Pod{pod(corev1.PodSucceeded, false), pod(corev1.PodRunning, true)}, true},
		{"succeeded and running not ready", []corev1.Pod{pod(corev1.PodSucceeded, false), pod(corev1.PodRunning, false)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := podsReady(tc.pods); got != tc.want {
				t.Errorf("podsReady = %v, want %v", got, tc.want)
			}
		})
	}
}
