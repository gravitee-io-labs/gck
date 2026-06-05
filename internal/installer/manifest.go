package installer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/logger"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

const (
	fieldManager = "gck"
)

// ManifestInstaller installs components from plain Kubernetes manifest files.
type ManifestInstaller struct{}

func getConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	return kubeconfig.ClientConfig()
}

func hasK8sWork(k8s *config.K8sSpec) bool {
	return len(k8s.ManifestFiles) > 0 || len(k8s.Manifests) > 0 ||
		len(k8s.Secrets) > 0 || len(k8s.ConfigMaps) > 0
}

func (m *ManifestInstaller) Install(ctx context.Context, comp config.Component, dir string, opts InstallOpts) error {
	if comp.K8s == nil {
		return fmt.Errorf("component %q has no k8s spec", comp.Name)
	}
	if !hasK8sWork(comp.K8s) {
		return fmt.Errorf("component %q has no manifest files, inline manifests, secrets, or configMaps", comp.Name)
	}

	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("kube config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("kubernetes client: %w", err)
	}
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("dynamic client: %w", err)
	}
	discoveryMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(clientset.Discovery()))

	namespace := comp.Namespace
	if namespace == "" {
		namespace = "default"
	}
	if namespace != "default" {
		_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create namespace %q: %w", namespace, err)
		}
	}

	if err := m.applyLocalResources(ctx, comp.K8s, namespace, discoveryMapper, dynClient, opts.DryRun, opts.DiffWriter); err != nil {
		return err
	}

	for _, f := range comp.K8s.ManifestFiles {
		path := filepath.Join(dir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %q: %w", path, err)
		}
		docs := splitYAMLDocuments(data)
		for _, doc := range docs {
			doc = strings.TrimSpace(doc)
			if doc == "" {
				continue
			}
			obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
			if err := yaml.Unmarshal([]byte(doc), &obj.Object); err != nil {
				return fmt.Errorf("decoding manifest in %q: %w", path, err)
			}
			if err := m.applyObject(ctx, obj, namespace, discoveryMapper, dynClient, opts.DryRun, opts.DiffWriter); err != nil {
				return err
			}
		}
	}

	for i, manifest := range comp.K8s.Manifests {
		obj := &unstructured.Unstructured{Object: manifest}
		if err := m.applyObject(ctx, obj, namespace, discoveryMapper, dynClient, opts.DryRun, opts.DiffWriter); err != nil {
			return fmt.Errorf("inline manifest [%d]: %w", i, err)
		}
	}

	return nil
}

func (m *ManifestInstaller) applyLocalResources(
	ctx context.Context,
	k8s *config.K8sSpec,
	namespace string,
	discoveryMapper meta.RESTMapper,
	dynClient dynamic.Interface,
	dryRun bool,
	diffWriter io.Writer,
) error {
	secrets, warnings, err := BuildSecrets(k8s.Secrets)
	for _, w := range warnings {
		logger.Warn("%s", w)
	}
	if err != nil {
		return err
	}
	for _, s := range secrets {
		if err := m.applyObject(ctx, s, namespace, discoveryMapper, dynClient, dryRun, diffWriter); err != nil {
			return fmt.Errorf("applying secret %q: %w", s.GetName(), err)
		}
	}

	configMaps, warnings, err := BuildConfigMaps(k8s.ConfigMaps)
	for _, w := range warnings {
		logger.Warn("%s", w)
	}
	if err != nil {
		return err
	}
	for _, cm := range configMaps {
		if err := m.applyObject(ctx, cm, namespace, discoveryMapper, dynClient, dryRun, diffWriter); err != nil {
			return fmt.Errorf("applying configmap %q: %w", cm.GetName(), err)
		}
	}

	return nil
}

func (m *ManifestInstaller) applyObject(
	ctx context.Context,
	obj *unstructured.Unstructured,
	namespace string,
	discoveryMapper meta.RESTMapper,
	dynClient dynamic.Interface,
	dryRun bool,
	diffWriter io.Writer,
) error {
	gvk := obj.GroupVersionKind()
	if gvk.Kind == "" {
		return nil
	}
	mapping, err := discoveryMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("mapping %s: %w", gvk, err)
	}
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		obj.SetNamespace(namespace)
	}
	gvr := mapping.Resource
	var ri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ri = dynClient.Resource(gvr).Namespace(obj.GetNamespace())
	} else {
		ri = dynClient.Resource(gvr)
	}

	var before string
	if dryRun && diffWriter != nil {
		live, err := ri.Get(ctx, obj.GetName(), metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("get %s %s: %w", gvk.Kind, obj.GetName(), err)
		}
		if live != nil {
			b, err := yaml.Marshal(live.Object)
			if err != nil {
				return fmt.Errorf("marshal live %s %s: %w", gvk.Kind, obj.GetName(), err)
			}
			before = string(b)
		}
	}

	applyOpts := metav1.ApplyOptions{FieldManager: fieldManager}
	if dryRun {
		applyOpts.DryRun = []string{"All"}
	}
	_, err = ri.Apply(ctx, obj.GetName(), obj, applyOpts)
	if err != nil {
		return fmt.Errorf("apply %s %s: %w", gvk.Kind, obj.GetName(), err)
	}

	if dryRun && diffWriter != nil {
		after, err := yaml.Marshal(obj.Object)
		if err != nil {
			return fmt.Errorf("marshal proposed %s %s: %w", gvk.Kind, obj.GetName(), err)
		}
		diffName := fmt.Sprintf("%s/%s", gvk.Kind, obj.GetName())
		if err := RenderDiff(diffName, before, string(after), diffWriter); err != nil {
			return fmt.Errorf("render diff %s: %w", diffName, err)
		}
	}

	return nil
}

func splitYAMLDocuments(data []byte) []string {
	var docs []string
	var buf strings.Builder
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if buf.Len() > 0 {
				docs = append(docs, buf.String())
				buf.Reset()
			}
			continue
		}
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
	}
	if buf.Len() > 0 {
		docs = append(docs, buf.String())
	}
	return docs
}

// Uninstall re-reads manifest files and deletes the resources.
func (m *ManifestInstaller) Uninstall(_ context.Context, comp config.Component) error {
	if comp.K8s == nil || !hasK8sWork(comp.K8s) {
		return nil
	}
	// Resolve dir: we don't have it in Uninstall. We need resolved.Dir in cmd.
	// For now we only support uninstall when we can resolve context again, or we
	// could require absolute paths in manifest files. The cmd layer doesn't pass dir to Uninstall.
	// So we cannot re-read files here. Return a clear error or skip delete.
	return fmt.Errorf("manifest uninstall not implemented (component %q): re-run with context to delete", comp.Name)
}
