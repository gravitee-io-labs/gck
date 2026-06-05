package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/kind"
	"github.com/gravitee-io-labs/gck/internal/state"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:         "list",
	Short:       "List all gck-managed clusters",
	Annotations: map[string]string{"gck_skip_config": "true"},
	RunE:        runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

type listRow struct {
	name    string
	created string
	from    string
	flags   string
	status  string
	running bool
}

func runList(_ *cobra.Command, _ []string) error {
	stateDir := filepath.Join(gckHome, "clusters")

	names, err := state.List(stateDir)
	if err != nil {
		return fmt.Errorf("listing cluster states: %w", err)
	}

	if len(names) == 0 {
		fmt.Println("No clusters found.")
		return nil
	}

	rows := make([]listRow, 0, len(names))
	nameW, createdW, fromW, flagsW := len("NAME"), len("CREATED"), len("FROM"), len("FLAGS")

	for _, name := range names {
		r := listRow{name: name}
		cs, err := state.Load(stateDir, name)
		if err != nil {
			r.status = "unknown"
			r.created = "-"
			r.from = "-"
			r.flags = "-"
		} else {
			r.name = cs.Name
			r.created = cs.CreatedAt.Format("2006-01-02 15:04")
			if len(cs.From) > 0 {
				r.from = cs.From[0]
				if len(cs.From) > 1 {
					r.from += fmt.Sprintf(" (+%d)", len(cs.From)-1)
				}
			} else {
				r.from = "-"
			}
			if len(cs.Flags) > 0 {
				flagStrs := make([]string, len(cs.Flags))
				for i, f := range cs.Flags {
					flagStrs[i] = "--" + f
				}
				r.flags = strings.Join(flagStrs, ", ")
			} else {
				r.flags = "-"
			}
			ok, kerr := kind.Exists(cs.Name)
			r.running = kerr == nil && ok
			if r.running {
				r.status = "running"
			} else {
				r.status = "stopped"
			}
		}
		if len(r.name) > nameW {
			nameW = len(r.name)
		}
		if len(r.created) > createdW {
			createdW = len(r.created)
		}
		if len(r.from) > fromW {
			fromW = len(r.from)
		}
		if len(r.flags) > flagsW {
			flagsW = len(r.flags)
		}
		rows = append(rows, r)
	}

	bold := color.New(color.Bold)
	fmtStr := fmt.Sprintf("%%-%ds   %%-%ds   %%-%ds   %%-%ds   %%s\n", nameW, createdW, fromW, flagsW)

	bold.Printf(fmtStr, "NAME", "CREATED", "FROM", "FLAGS", "STATUS")
	for _, r := range rows {
		var status string
		switch r.status {
		case "running":
			status = color.BlueString(r.status)
		default:
			status = color.YellowString(r.status)
		}
		fmt.Printf(fmtStr, r.name, r.created, r.from, r.flags, status)
	}

	return nil
}
