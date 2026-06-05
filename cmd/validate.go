package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/logger"
	"github.com/gravitee-io-labs/gck/internal/registry"
	internalschema "github.com/gravitee-io-labs/gck/internal/schema"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate [path...]",
	Short: "Validate gck.yaml and context flag files against the configuration schema",
	Long: `Validate one or more gck.yaml files against the gck configuration schema.

Each argument can be a path to a gck.yaml file or a directory. When a
directory is given, all gck.yaml and gck--*.yaml (context flag) files
under it are validated recursively. Context flag files are additionally
checked for a valid naming convention and a non-empty description field.

When --tags is provided with a path to a tags vocabulary file, README.md
files that sit alongside a gck.yaml are also checked: every tag in the
README's YAML frontmatter must belong to the allowed set. When --tags is
omitted, tag validation is skipped entirely.

When no argument is given, validates ./gck.yaml in the current directory.`,
	RunE: runValidate,
}

var tagsFile string

func init() {
	validateCmd.Flags().StringVar(&tagsFile, "tags", "", "path to a tags vocabulary file for README tag validation")
	rootCmd.AddCommand(validateCmd)
}

func isFlagFile(name string) bool {
	return strings.HasPrefix(name, "gck--") && strings.HasSuffix(name, ".yaml")
}

func runValidate(_ *cobra.Command, args []string) error {
	sch, err := internalschema.Compile(SchemaData)
	if err != nil {
		return fmt.Errorf("compiling schema: %w", err)
	}

	targets := args
	if len(targets) == 0 {
		targets = []string{"gck.yaml"}
	}

	// gckDirs tracks directories that contain a gck.yaml so we can locate
	// README.md files sitting alongside them for tag validation.
	gckDirs := make(map[string]bool)

	var configFiles []string
	var flagFiles []string
	var badSegments []string
	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("cannot access %s: %w", target, err)
		}
		if !info.IsDir() {
			if isFlagFile(filepath.Base(target)) {
				flagFiles = append(flagFiles, target)
			} else {
				configFiles = append(configFiles, target)
			}
			continue
		}
		if err := filepath.Walk(target, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				segment := fi.Name()
				if strings.Contains(segment, ".") && segment != "." {
					badSegments = append(badSegments, path)
				}
				return nil
			}
			name := fi.Name()
			if name == "gck.yaml" {
				configFiles = append(configFiles, path)
				gckDirs[filepath.Dir(path)] = true
			} else if isFlagFile(name) {
				flagFiles = append(flagFiles, path)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("walking %s: %w", target, err)
		}
	}

	var failed int
	for _, seg := range badSegments {
		logger.Error("%s: directory name contains a dot (registry path segments must not contain dots)", seg)
		failed++
	}

	total := len(configFiles) + len(flagFiles)
	if total == 0 && failed == 0 {
		return fmt.Errorf("no gck.yaml or flag files found")
	}
	for _, f := range configFiles {
		if err := internalschema.ValidateFile(sch, f); err != nil {
			logger.Error("%s: %v", f, err)
			failed++
		}
	}
	for _, f := range flagFiles {
		if err := validateFlagFile(sch, f); err != nil {
			logger.Error("%s: %v", f, err)
			failed++
		}
	}

	var readmeCount int
	if tagsFile != "" {
		allowed, err := registry.LoadTags(tagsFile)
		if err != nil {
			return fmt.Errorf("loading tags: %w", err)
		}
		for dir := range gckDirs {
			readme := filepath.Join(dir, "README.md")
			if _, err := os.Stat(readme); err != nil {
				continue
			}
			readmeCount++
			if err := registry.ValidateReadmeTags(readme, allowed); err != nil {
				logger.Error("%s: %v", readme, err)
				failed++
			}
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d file(s) failed validation", failed)
	}

	logger.Success("%d file(s) valid", len(configFiles)+len(flagFiles)+readmeCount)
	return nil
}

// validateFlagFile validates a context flag file: naming convention,
// description required, and schema compliance.
func validateFlagFile(sch *jsonschema.Schema, path string) error {
	name := filepath.Base(path)
	if _, err := registry.FlagNameFromFile(name); err != nil {
		return err
	}

	if err := internalschema.ValidateFile(sch, path); err != nil {
		return fmt.Errorf("schema: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	if err := registry.ValidateFlagDescription(data); err != nil {
		return err
	}

	return nil
}
