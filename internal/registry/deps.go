package registry

import (
	"errors"
	"fmt"

	"github.com/gravitee-io-labs/gck/internal/config"
)

func Validate(components []config.Component) error {
	nameToComp := make(map[string]config.Component)
	for _, c := range components {
		nameToComp[c.Name] = c
	}
	for _, c := range components {
		for _, req := range c.Requires {
			if _, ok := nameToComp[req.Component]; !ok {
				return fmt.Errorf("component %q requires unknown component %q", c.Name, req.Component)
			}
		}
	}
	_, err := TopoSort(components)
	return err
}

func TopoSort(components []config.Component) ([]config.Component, error) {
	nameToComp := make(map[string]config.Component)
	for _, c := range components {
		nameToComp[c.Name] = c
	}

	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	for name := range nameToComp {
		inDegree[name] = 0
	}
	for _, c := range components {
		for _, req := range c.Requires {
			adj[req.Component] = append(adj[req.Component], c.Name)
			inDegree[c.Name]++
		}
	}

	var queue []string
	for name, d := range inDegree {
		if d == 0 {
			queue = append(queue, name)
		}
	}

	var order []config.Component
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		order = append(order, nameToComp[name])
		for _, next := range adj[name] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(order) != len(components) {
		return nil, errors.New("dependency cycle detected among components")
	}
	return order, nil
}
