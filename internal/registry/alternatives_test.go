package registry

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
)

// writeAltRegistry lays out a small registry with one product context that
// declares a "datasource" group (pg by default, mongo as the other member),
// each member composing its own database context.
func writeAltRegistry(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "db", "pg", "gck.yaml"), `
vars:
  imageTag:
    default: "latest"
components:
  - name: pg
    helm:
      chart: db/pg
      values:
        tag: "{{ .imageTag }}"
`)
	writeFile(t, filepath.Join(root, "db", "mongo", "gck.yaml"), `
components:
  - name: mongo
    helm:
      chart: db/mongo
`)
	writeFile(t, filepath.Join(root, "product", "gck.yaml"), `
components:
  - name: app
    helm:
      chart: app/chart
      values:
        store: none
        name: app
`)
	writeFile(t, filepath.Join(root, "product", "gck--use-pg.yaml"), `
description: "Store data in pg"
group: datasource
default: true
from:
  - db/pg
vars:
  db:
    pg:
      imageTag:
        default: "17"
  storeUrl:
    default: "pg://pg:5432"
components:
  - name: app
    helm:
      values:
        store: "{{ .storeUrl }}"
`)
	writeFile(t, filepath.Join(root, "product", "gck--use-mongo.yaml"), `
description: "Store data in mongo"
group: datasource
from:
  - db/mongo
components:
  - name: app
    helm:
      values:
        store: mongo
`)
	writeFile(t, filepath.Join(root, "product", "gck--enable-debug.yaml"), `
description: "Debug mode"
requires:
  - use-pg
components:
  - name: app
    helm:
      values:
        debug: true
`)
	return root
}

func componentNames(r *config.ResolvedContext) []string {
	names := make([]string, 0, len(r.Components))
	for _, c := range r.Components {
		names = append(names, c.Name)
	}
	return names
}

func componentValues(t *testing.T, r *config.ResolvedContext, name string) map[string]any {
	t.Helper()
	for _, c := range r.Components {
		if c.Name == name && c.Helm != nil {
			return c.Helm.Values
		}
	}
	t.Fatalf("component %q not found in %v", name, componentNames(r))
	return nil
}

func TestAlternatives_DefaultMemberApplied(t *testing.T) {
	root := writeAltRegistry(t)

	resolved, err := (&FSResolver{Root: root}).Resolve(context.Background(), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The member's from comes ahead of the context's own composition, as a
	// dedicated variant context would have declared it.
	if got, want := componentNames(resolved), []string{"pg", "app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("components = %v, want %v", got, want)
	}
	// The member's body wins over the context's own values.
	if got := componentValues(t, resolved, "app")["store"]; got != "pg://pg:5432" {
		t.Fatalf("store = %v, want the pg member's value", got)
	}
	if got := componentValues(t, resolved, "app")["name"]; got != "app" {
		t.Fatalf("name = %v, want the context's own value kept", got)
	}
	if got := resolved.Selected["datasource"]; got != "use-pg" {
		t.Fatalf("Selected[datasource] = %q, want use-pg", got)
	}
}

func TestAlternatives_PathScopedVarReachesMemberParent(t *testing.T) {
	root := writeAltRegistry(t)

	resolved, err := (&FSResolver{Root: root}).Resolve(context.Background(), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := componentValues(t, resolved, "pg")["tag"]; got != "17" {
		t.Fatalf("pg tag = %v, want the member's path-scoped pin 17", got)
	}
}

func TestAlternatives_ExplicitSelection(t *testing.T) {
	root := writeAltRegistry(t)

	ctx := WithUse(context.Background(), []string{"use-mongo"})
	resolved, err := (&FSResolver{Root: root}).Resolve(ctx, "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := componentNames(resolved), []string{"mongo", "app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("components = %v, want %v: the default member must not be applied", got, want)
	}
	if got := componentValues(t, resolved, "app")["store"]; got != "mongo" {
		t.Fatalf("store = %v, want mongo", got)
	}
	if got := resolved.Selected["datasource"]; got != "use-mongo" {
		t.Fatalf("Selected[datasource] = %q, want use-mongo", got)
	}
}

func TestAlternatives_MemberNameWithoutPrefix(t *testing.T) {
	root := writeAltRegistry(t)

	ctx := WithUse(context.Background(), []string{"mongo"})
	resolved, err := (&FSResolver{Root: root}).Resolve(ctx, "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resolved.Selected["datasource"]; got != "use-mongo" {
		t.Fatalf("Selected[datasource] = %q, want use-mongo", got)
	}
}

func TestAlternatives_ConflictWithinGroup(t *testing.T) {
	root := writeAltRegistry(t)

	ctx := WithUse(context.Background(), []string{"use-pg", "use-mongo"})
	_, err := (&FSResolver{Root: root}).Resolve(ctx, "product")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected a mutually exclusive error, got %v", err)
	}
}

func TestAlternatives_ConfiguredSelection(t *testing.T) {
	root := writeAltRegistry(t)

	ctx := WithConfiguredUse(context.Background(), []string{"mongo"})
	resolved, err := (&FSResolver{Root: root}).Resolve(ctx, "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resolved.Selected["datasource"]; got != "use-mongo" {
		t.Fatalf("Selected[datasource] = %q, want use-mongo", got)
	}
}

// The command line overrides the configuration's use: in the same group,
// as it overrides --from and --registry.
func TestAlternatives_CommandLineOverridesConfiguredSelection(t *testing.T) {
	root := writeAltRegistry(t)

	ctx := WithUse(WithConfiguredUse(context.Background(), []string{"mongo"}), []string{"use-pg"})
	resolved, err := (&FSResolver{Root: root}).Resolve(ctx, "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resolved.Selected["datasource"]; got != "use-pg" {
		t.Fatalf("Selected[datasource] = %q, want use-pg from the command line", got)
	}
}

func TestAlternatives_ConfiguredConflictWithinGroup(t *testing.T) {
	root := writeAltRegistry(t)

	ctx := WithConfiguredUse(context.Background(), []string{"pg", "mongo"})
	_, err := (&FSResolver{Root: root}).Resolve(ctx, "product")
	if err == nil || !strings.Contains(err.Error(), "use: ") || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected a use: mutually exclusive error, got %v", err)
	}

	// A command-line choice settles the configuration's conflict.
	ctx = WithUse(ctx, []string{"use-mongo"})
	if _, err := (&FSResolver{Root: root}).Resolve(ctx, "product"); err != nil {
		t.Fatalf("the command line should win over the configuration: %v", err)
	}
}

func TestAlternatives_PinHidesGroupAndRejectsOverride(t *testing.T) {
	root := writeAltRegistry(t)
	writeFile(t, filepath.Join(root, "app-on-mongo", "gck.yaml"), `
from:
  - product
use:
  - mongo
`)
	r := &FSResolver{Root: root}

	resolved, err := r.Resolve(context.Background(), "app-on-mongo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resolved.Selected["datasource"]; got != "use-mongo" {
		t.Fatalf("Selected[datasource] = %q, want the pinned use-mongo", got)
	}
	for _, f := range resolved.Flags {
		if f.IsAlternative() && !f.Pinned {
			t.Fatalf("alternative --%s should be marked pinned", f.Name)
		}
	}

	// Selecting the pinned member again is harmless.
	if _, err := r.Resolve(WithUse(context.Background(), []string{"use-mongo"}), "app-on-mongo"); err != nil {
		t.Fatalf("selecting the pinned member: %v", err)
	}

	_, err = r.Resolve(WithUse(context.Background(), []string{"use-pg"}), "app-on-mongo")
	if err == nil || !strings.Contains(err.Error(), "pins group datasource") {
		t.Fatalf("expected a pin error, got %v", err)
	}
}

func TestAlternatives_UnknownPin(t *testing.T) {
	root := writeAltRegistry(t)
	writeFile(t, filepath.Join(root, "bad", "gck.yaml"), `
from:
  - product
use:
  - oracle
`)

	_, err := (&FSResolver{Root: root}).Resolve(context.Background(), "bad")
	if err == nil || !strings.Contains(err.Error(), "use: oracle matches no alternative") {
		t.Fatalf("expected an unknown pin error, got %v", err)
	}
}

func TestAlternatives_ChildCannotRedeclareInheritedGroup(t *testing.T) {
	root := writeAltRegistry(t)
	writeFile(t, filepath.Join(root, "child", "gck.yaml"), `
from:
  - product
`)
	writeFile(t, filepath.Join(root, "child", "gck--use-sqlite.yaml"), `
description: "sqlite"
group: datasource
default: true
`)

	_, err := (&FSResolver{Root: root}).Resolve(context.Background(), "child")
	if err == nil || !strings.Contains(err.Error(), "redeclares inherited alternative group datasource") {
		t.Fatalf("expected a redeclaration error, got %v", err)
	}
}

func TestAlternatives_InheritedByComposingContext(t *testing.T) {
	root := writeAltRegistry(t)
	writeFile(t, filepath.Join(root, "ee", "gck.yaml"), `
from:
  - product
components:
  - name: license
    helm:
      chart: ee/license
`)

	ctx := WithUse(context.Background(), []string{"use-mongo"})
	resolved, err := (&FSResolver{Root: root}).Resolve(ctx, "ee")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := componentNames(resolved), []string{"mongo", "app", "license"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("components = %v, want %v", got, want)
	}
}

func TestDiscoverFlags_AlternativeRules(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "use- flag without group",
			files: map[string]string{"gck--use-x.yaml": "description: x\ndefault: true\n"},
			want:  "must declare a group",
		},
		{
			name:  "plain flag with group",
			files: map[string]string{"gck--enable-x.yaml": "description: x\ngroup: g\n"},
			want:  "only use-* flags may declare a group",
		},
		{
			name:  "alternative with requires",
			files: map[string]string{"gck--use-x.yaml": "description: x\ngroup: g\ndefault: true\nrequires:\n  - enable-y\n"},
			want:  "alternatives may not declare requires",
		},
		{
			name: "group without default",
			files: map[string]string{
				"gck--use-a.yaml": "description: a\ngroup: g\n",
				"gck--use-b.yaml": "description: b\ngroup: g\n",
			},
			want: "exactly one default member, found 0",
		},
		{
			name: "group with two defaults",
			files: map[string]string{
				"gck--use-a.yaml": "description: a\ngroup: g\ndefault: true\n",
				"gck--use-b.yaml": "description: b\ngroup: g\ndefault: true\n",
			},
			want: "exactly one default member, found 2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tc.files {
				writeFile(t, filepath.Join(dir, name), content)
			}
			_, err := DiscoverFlags(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestApplyFlags_RequiresSelectedAlternative(t *testing.T) {
	root := writeAltRegistry(t)
	r := &FSResolver{Root: root}

	resolved, err := r.Resolve(context.Background(), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// use-pg is the default: it satisfies enable-debug without being passed.
	if err := ApplyFlags(resolved, []string{"enable-debug"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := componentValues(t, resolved, "app")["debug"]; got != true {
		t.Fatalf("debug = %v, want true", got)
	}

	resolved, err = r.Resolve(WithUse(context.Background(), []string{"use-mongo"}), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = ApplyFlags(resolved, []string{"use-mongo", "enable-debug"}, nil)
	if err == nil || !strings.Contains(err.Error(), "--enable-debug requires --use-pg") {
		t.Fatalf("expected a requires error, got %v", err)
	}
}

func TestApplyFlags_SkipsAlternatives(t *testing.T) {
	root := writeAltRegistry(t)

	resolved, err := (&FSResolver{Root: root}).Resolve(WithUse(context.Background(), []string{"use-mongo"}), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	before := len(resolved.Components)
	if err := ApplyFlags(resolved, []string{"use-mongo"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Components) != before {
		t.Fatalf("an alternative must not be applied a second time as a patch")
	}
}

func TestEffectiveFlags(t *testing.T) {
	resolved := &config.ResolvedContext{Selected: map[string]string{
		"datasource": "use-mongo",
		"analytics":  "use-es",
	}}
	got := EffectiveFlags(resolved, []string{"enable-a", "use-mongo", "disable-b", "enable-a"})
	want := []string{"use-es", "use-mongo", "enable-a", "disable-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EffectiveFlags = %v, want %v", got, want)
	}
}

func TestHTTPResolver_AlternativesAppliedOnceWhereDeclared(t *testing.T) {
	root := writeAltRegistry(t)
	writeFile(t, filepath.Join(root, "product", "gck.flags.yaml"), `
flags:
  - name: enable-debug
    source: product
  - name: use-mongo
    source: product
  - name: use-pg
    source: product
`)
	// The site publishes inherited flag files into every child directory and
	// lists them in the child's manifest; the source keeps the child from
	// applying the parent's alternatives a second time.
	writeFile(t, filepath.Join(root, "ee", "gck.yaml"), `
from:
  - product
`)
	writeFile(t, filepath.Join(root, "ee", "gck.flags.yaml"), `
flags:
  - name: enable-debug
    source: product
  - name: use-mongo
    source: product
  - name: use-pg
    source: product
`)
	for _, name := range []string{"gck--use-mongo.yaml", "gck--use-pg.yaml", "gck--enable-debug.yaml"} {
		data, err := os.ReadFile(filepath.Join(root, "product", name))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, "ee", name), string(data))
	}

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

	resolved, err := resolver.Resolve(WithUse(context.Background(), []string{"use-mongo"}), "ee")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := componentNames(resolved), []string{"mongo", "app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("components = %v, want %v", got, want)
	}
	if len(resolved.Flags) != 3 {
		t.Fatalf("expected the 3 flags declared by product, got %d", len(resolved.Flags))
	}
	if got := resolved.Selected["datasource"]; got != "use-mongo" {
		t.Fatalf("Selected[datasource] = %q, want use-mongo", got)
	}
}

// writeCascadeRegistry lays out a product with a datasource group (pg by
// default, or none) and an analytics group (es by default). Selecting "none"
// implies disable-ui and disable-analytics; disable-analytics disables the
// analytics group; enable-console-sso conflicts with use-none.
func writeCascadeRegistry(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, db := range []string{"pg", "es"} {
		writeFile(t, filepath.Join(root, "db", db, "gck.yaml"), `
components:
  - name: `+db+`
    helm:
      chart: db/`+db+`
`)
	}
	writeFile(t, filepath.Join(root, "product", "gck.yaml"), `
vars:
  imagePrefix:
    default: "acme"
components:
  - name: app
    helm:
      chart: app/chart
      values:
        ui: true
`)
	writeFile(t, filepath.Join(root, "product", "gck--use-pg.yaml"), `
description: "pg"
group: datasource
default: true
from:
  - db/pg
`)
	// References a var the declaring context declares, like a plain flag.
	writeFile(t, filepath.Join(root, "product", "gck--use-none.yaml"), `
description: "no datasource"
group: datasource
implies:
  - disable-ui
  - disable-analytics
components:
  - name: app
    helm:
      values:
        image: "{{ .imagePrefix }}/gateway"
`)
	writeFile(t, filepath.Join(root, "product", "gck--use-es.yaml"), `
description: "es"
group: analytics
default: true
from:
  - db/es
`)
	writeFile(t, filepath.Join(root, "product", "gck--disable-analytics.yaml"), `
description: "no analytics"
disables:
  - analytics
components:
  - name: app
    helm:
      values:
        analytics: false
`)
	writeFile(t, filepath.Join(root, "product", "gck--disable-ui.yaml"), `
description: "no ui"
components:
  - name: app
    helm:
      values:
        ui: false
`)
	writeFile(t, filepath.Join(root, "product", "gck--enable-console-sso.yaml"), `
description: "sso"
conflicts:
  - use-none
components: []
`)
	return root
}

func TestAlternatives_DisablesKeepsGroupOutOfComposition(t *testing.T) {
	root := writeCascadeRegistry(t)
	r := &FSResolver{Root: root}

	resolved, err := r.Resolve(context.Background(), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := componentNames(resolved), []string{"es", "pg", "app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("defaults: components = %v, want %v", got, want)
	}

	resolved, err = r.Resolve(WithFlags(context.Background(), []string{"disable-analytics"}), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := componentNames(resolved), []string{"pg", "app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("components = %v, want %v: a disabled group's member must not be composed", got, want)
	}
	if _, ok := resolved.Selected["analytics"]; ok {
		t.Fatalf("Selected = %v, want no analytics entry", resolved.Selected)
	}
}

func TestAlternatives_ImpliesCascades(t *testing.T) {
	root := writeCascadeRegistry(t)

	resolved, err := (&FSResolver{Root: root}).Resolve(WithUse(context.Background(), []string{"none"}), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// use-none implies disable-analytics, which disables the analytics group.
	if got, want := componentNames(resolved), []string{"app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("components = %v, want %v", got, want)
	}
	if got := componentValues(t, resolved, "app")["image"]; got != "acme/gateway" {
		t.Fatalf("image = %v, want the alternative rendered with its context's vars", got)
	}
	if want := []string{"disable-ui", "disable-analytics"}; !reflect.DeepEqual(resolved.Implied, want) {
		t.Fatalf("Implied = %v, want %v", resolved.Implied, want)
	}
	if got, want := EffectiveFlags(resolved, []string{"disable-ui"}), []string{"use-none", "disable-ui", "disable-analytics"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("EffectiveFlags = %v, want %v", got, want)
	}

	// Implied flags are applied without being passed.
	if err := ApplyFlags(resolved, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	values := componentValues(t, resolved, "app")
	if values["ui"] != false || values["analytics"] != false {
		t.Fatalf("values = %v, want ui and analytics off", values)
	}
}

func TestApplyFlags_Conflicts(t *testing.T) {
	root := writeCascadeRegistry(t)
	r := &FSResolver{Root: root}

	resolved, err := r.Resolve(context.Background(), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ApplyFlags(resolved, []string{"enable-console-sso"}, nil); err != nil {
		t.Fatalf("no conflict with the default datasource: %v", err)
	}

	resolved, err = r.Resolve(WithUse(context.Background(), []string{"none"}), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = ApplyFlags(resolved, []string{"enable-console-sso"}, nil)
	if err == nil || !strings.Contains(err.Error(), "--enable-console-sso cannot be used with --use-none") {
		t.Fatalf("expected a conflict error, got %v", err)
	}
}

func TestDiscoverFlags_CascadeFieldRules(t *testing.T) {
	cases := []struct {
		name, file, content, want string
	}{
		{"plain flag with implies", "gck--enable-x.yaml", "description: x\nimplies:\n  - disable-y\n", "only alternatives (use-*) may declare implies"},
		{"alternative with disables", "gck--use-x.yaml", "description: x\ngroup: g\ndefault: true\ndisables:\n  - h\n", "alternatives may not declare disables"},
		{"alternative with conflicts", "gck--use-x.yaml", "description: x\ngroup: g\ndefault: true\nconflicts:\n  - use-y\n", "alternatives may not declare conflicts"},
		{"implies an alternative", "gck--use-x.yaml", "description: x\ngroup: g\ndefault: true\nimplies:\n  - use-y\n", "implies names plain flags"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, tc.file), tc.content)
			_, err := DiscoverFlags(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestAlternatives_ImpliedFlagMustExist(t *testing.T) {
	root := writeCascadeRegistry(t)
	writeFile(t, filepath.Join(root, "product", "gck--use-none.yaml"), `
description: "no datasource"
group: datasource
implies:
  - disable-everything
`)

	_, err := (&FSResolver{Root: root}).Resolve(WithUse(context.Background(), []string{"none"}), "product")
	if err == nil || !strings.Contains(err.Error(), "implies --disable-everything") {
		t.Fatalf("expected an unknown implied flag error, got %v", err)
	}
}

func TestPlainFlagFrom_ComposesOnlyWhenActive(t *testing.T) {
	root := writeCascadeRegistry(t)
	writeFile(t, filepath.Join(root, "broker", "gck.yaml"), `
vars:
  imageTag:
    default: "latest"
components:
  - name: broker
    helm:
      chart: broker/chart
      values:
        tag: "{{ .imageTag }}"
`)
	writeFile(t, filepath.Join(root, "product", "gck--enable-streaming.yaml"), `
description: "streaming"
from:
  - broker
vars:
  broker:
    imageTag:
      default: "3.9"
components:
  - name: app
    helm:
      values:
        streaming: true
`)
	r := &FSResolver{Root: root}

	resolved, err := r.Resolve(context.Background(), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, name := range componentNames(resolved) {
		if name == "broker" {
			t.Fatal("an inactive flag must not compose its parents")
		}
	}

	resolved, err = r.Resolve(WithFlags(context.Background(), []string{"enable-streaming"}), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := componentNames(resolved), []string{"es", "pg", "broker", "app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("components = %v, want %v", got, want)
	}
	if got := componentValues(t, resolved, "broker")["tag"]; got != "3.9" {
		t.Fatalf("broker tag = %v, want the flag's path-scoped pin 3.9", got)
	}
	// The flag's patch is not applied at resolution...
	if _, ok := componentValues(t, resolved, "app")["streaming"]; ok {
		t.Fatal("the flag's body must be left to ApplyFlags")
	}
	// ...but by ApplyFlags, once.
	if err := ApplyFlags(resolved, []string{"enable-streaming"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := componentValues(t, resolved, "app")["streaming"]; got != true {
		t.Fatalf("streaming = %v, want true", got)
	}
}
