// Package build implements the local Docker image build pipeline for the
// inner dev loop: compile, build, push to the preload registry, and restart workloads.
package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gravitee-io-labs/gck/internal/cache"
	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/logger"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
)

// Options configures the build pipeline.
type Options struct {
	ClusterName string
	GckHome     string
	SkipPre     bool
	NoRestart   bool
	LogWriter   io.Writer
}

// Run executes the full build pipeline for a single Build entry:
// env expansion, pre-build commands, Docker build, push to preload registry, and rollout restart.
func Run(ctx context.Context, b config.Build, opts Options) error {
	dir := b.Dir
	if dir == "" {
		dir = "."
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving build directory: %w", err)
	}

	logw := opts.LogWriter
	if logw == nil {
		logw = io.Discard
	}

	if !opts.SkipPre {
		if err := logger.WithSpinner(
			"Running pre-build commands",
			func() error {
				for _, c := range b.Pre {
					if err := runCommand(dir, c, logw); err != nil {
						return err
					}
				}
				return nil
			},
		); err != nil {
			return err
		}
	}

	if shouldDockerBuild(dir, b.Context, b.Dockerfile) {
		if err := logger.WithSpinner(
			fmt.Sprintf("Building image %s", b.Image),
			func() error {
				return dockerBuild(ctx, dir, b.Context, b.Dockerfile, b.Image, b.Platform, buildArgPointers(b.BuildArgs), logw)
			},
		); err != nil {
			return err
		}
	}

	if err := logger.WithSpinner(
		"Pushing image to preload registry",
		func() error {
			if err := cache.EnsurePreloadRegistry(ctx, opts.GckHome); err != nil {
				return fmt.Errorf("ensuring preload registry: %w", err)
			}
			if err := cache.ConnectPreloadToKindNetwork(ctx); err != nil {
				return fmt.Errorf("connecting preload registry to kind network: %w", err)
			}
			return cache.PushImages(ctx, []string{b.Image})
		},
	); err != nil {
		return err
	}

	if !opts.NoRestart {
		return restartWorkloads(ctx, b.Image, opts.ClusterName)
	}
	return nil
}

func buildArgPointers(args map[string]string) map[string]*string {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]*string, len(args))
	for k, v := range args {
		v := v
		out[k] = &v
	}
	return out
}

func runCommand(dir, command string, w io.Writer) error {
	fmt.Fprintf(w, "\n  $ %s\n\n", command)
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", command, err)
	}
	fmt.Fprintln(w)
	return nil
}

func shouldDockerBuild(dir, buildContext, dockerfile string) bool {
	if dockerfile != "" {
		return true
	}
	contextDir := dir
	if buildContext != "" {
		contextDir = filepath.Join(dir, buildContext)
	}
	_, err := os.Stat(filepath.Join(contextDir, "Dockerfile"))
	return err == nil
}

func dockerBuild(ctx context.Context, dir, buildContext, dockerfile, imageName, platform string, buildArgs map[string]*string, logw io.Writer) error {
	contextDir := dir
	if buildContext != "" {
		contextDir = filepath.Join(dir, buildContext)
	}

	args := []string{"build", "-t", imageName}

	if platform != "" {
		args = append(args, "--platform", platform)
	}

	if dockerfile != "" {
		args = append(args, "-f", filepath.Join(dir, dockerfile))
	}

	for k, v := range buildArgs {
		if v != nil {
			args = append(args, "--build-arg", fmt.Sprintf("%s=%s", k, *v))
		}
	}

	args = append(args, contextDir)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")
	cmd.Stdout = logw
	cmd.Stderr = logw

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}
	return nil
}

// restartWorkloads finds Deployments and StatefulSets that reference the
// given image and performs a rollout restart on each.
func restartWorkloads(ctx context.Context, imageName, clusterName string) error {
	restCfg, err := clusterRESTConfig(clusterName)
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	patch := []byte(fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`,
		time.Now().Format(time.RFC3339),
	))

	var restarted int

	deploys, err := clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing deployments: %w", err)
	}
	for i := range deploys.Items {
		d := &deploys.Items[i]
		if !podSpecReferencesImage(d.Spec.Template.Spec, imageName) {
			continue
		}
		if _, err := clientset.AppsV1().Deployments(d.Namespace).Patch(
			ctx, d.Name, k8stypes.StrategicMergePatchType, patch, metav1.PatchOptions{},
		); err != nil {
			return fmt.Errorf("restarting deployment %s/%s: %w", d.Namespace, d.Name, err)
		}
		logger.Success("Restarted deployment %s/%s", d.Namespace, d.Name)
		restarted++
	}

	stsets, err := clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing statefulsets: %w", err)
	}
	for i := range stsets.Items {
		s := &stsets.Items[i]
		if !podSpecReferencesImage(s.Spec.Template.Spec, imageName) {
			continue
		}
		if _, err := clientset.AppsV1().StatefulSets(s.Namespace).Patch(
			ctx, s.Name, k8stypes.StrategicMergePatchType, patch, metav1.PatchOptions{},
		); err != nil {
			return fmt.Errorf("restarting statefulset %s/%s: %w", s.Namespace, s.Name, err)
		}
		logger.Success("Restarted statefulset %s/%s", s.Namespace, s.Name)
		restarted++
	}

	if restarted == 0 {
		logger.Warn("No workloads found using image %s", imageName)
	}

	return nil
}

func podSpecReferencesImage(spec v1.PodSpec, imageName string) bool {
	for _, c := range spec.Containers {
		if c.Image == imageName {
			return true
		}
	}
	for _, c := range spec.InitContainers {
		if c.Image == imageName {
			return true
		}
	}
	return false
}

func clusterRESTConfig(clusterName string) (*rest.Config, error) {
	provider := cluster.NewProvider()
	kubeConfig, err := provider.KubeConfig(clusterName, false)
	if err != nil {
		return nil, fmt.Errorf("getting kubeconfig for %q: %w", clusterName, err)
	}
	restCfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfig))
	if err != nil {
		return nil, fmt.Errorf("parsing kubeconfig: %w", err)
	}
	return restCfg, nil
}
