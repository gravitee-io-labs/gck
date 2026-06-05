package cache

import (
	"context"
	"fmt"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const kindNetworkName = "kind"

// ConnectToKindNetwork connects every cache proxy container to the Kind Docker
// network so that Kind nodes can resolve them by container name.
func ConnectToKindNetwork(ctx context.Context, _ string, cfg *config.MirrorsConfig) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()

	netID, err := findKindNetwork(ctx, cli)
	if err != nil {
		return err
	}

	inspect, err := cli.NetworkInspect(ctx, netID, network.InspectOptions{})
	if err != nil {
		return fmt.Errorf("inspecting network %q: %w", kindNetworkName, err)
	}

	connected := make(map[string]bool, len(inspect.Containers))
	for _, ep := range inspect.Containers {
		connected[ep.Name] = true
	}

	for _, upstream := range AllUpstreams(cfg) {
		name := ContainerName(upstream)
		if connected[name] {
			continue
		}
		if err := cli.NetworkConnect(ctx, netID, name, nil); err != nil {
			return fmt.Errorf("connecting %q to kind network: %w", name, err)
		}
	}
	return nil
}

// DisconnectFromKindNetwork disconnects every cache proxy container from the
// Kind Docker network. Silently succeeds if the network no longer exists.
func DisconnectFromKindNetwork(ctx context.Context, _ string, cfg *config.MirrorsConfig) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()

	netID, err := findKindNetwork(ctx, cli)
	if err != nil {
		return nil
	}

	inspect, err := cli.NetworkInspect(ctx, netID, network.InspectOptions{})
	if err != nil {
		return nil
	}

	connected := make(map[string]bool, len(inspect.Containers))
	for _, ep := range inspect.Containers {
		connected[ep.Name] = true
	}

	for _, upstream := range AllUpstreams(cfg) {
		name := ContainerName(upstream)
		if !connected[name] {
			continue
		}
		if err := cli.NetworkDisconnect(ctx, netID, name, true); err != nil {
			return fmt.Errorf("disconnecting %q from kind network: %w", name, err)
		}
	}
	return nil
}

func findKindNetwork(ctx context.Context, cli *client.Client) (string, error) {
	nets, err := cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("listing docker networks: %w", err)
	}
	for _, n := range nets {
		if n.Name == kindNetworkName {
			return n.ID, nil
		}
	}
	return "", fmt.Errorf("kind network %q not found", kindNetworkName)
}
