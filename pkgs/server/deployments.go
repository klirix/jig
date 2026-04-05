package main

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
	"github.com/go-chi/chi/v5"
	"github.com/goccy/go-yaml"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func makeEnvs(newenvs map[string]string, secretDb *Secrets) ([]string, error) {
	resolvedEnvs := []string{}
	for key, value := range newenvs {
		if strings.HasPrefix(value, "@") {
			secretValue, found, err := secretDb.Get(value[1:])
			if !found {
				return nil, errors.New("Secret not found: " + value)
			}
			if err != nil {
				return nil, errors.New("Failed to get secret value: " + value)
			}
			value = secretValue
		}
		resolvedEnvs = append(resolvedEnvs, key+"="+value)
	}
	return resolvedEnvs, nil
}

func makeEnvMap(newenvs map[string]string, secretDb *Secrets) (map[string]string, error) {
	resolvedEnvs, err := makeEnvs(newenvs, secretDb)
	if err != nil {
		return nil, err
	}

	envMap := make(map[string]string, len(resolvedEnvs))
	for _, env := range resolvedEnvs {
		key, value, found := strings.Cut(env, "=")
		if !found {
			return nil, fmt.Errorf("invalid environment value %q", env)
		}
		envMap[key] = value
	}

	return envMap, nil
}

func makeRule(config jigtypes.DeploymentConfig) string {
	switch {
	case config.Rule != "":
		return config.Rule
	case config.Domain != "":
		return "Host(`" + config.Domain + "`)"
	default:
		return "No-HTTP"
	}
}

type deploymentBackend string

const (
	deploymentBackendContainers deploymentBackend = "containers"
	deploymentBackendSwarm      deploymentBackend = "swarm"
)

func makeLabels(config jigtypes.DeploymentConfig) map[string]string {
	return makeRoutingLabels(config)
}

func makeRoutingLabels(config jigtypes.DeploymentConfig) map[string]string {
	name := config.Name
	rule := makeRule(config)

	var configString string
	if configStringBytes, err := json.Marshal(config); err != nil {
		configString = ""
	} else {
		configString = string(configStringBytes)
	}

	labels := map[string]string{
		"traefik.docker.network": "jig",
		"jig.name":               name,
		"jig.config":             configString,
	}
	if config.Port != 0 {
		labels["traefik.http.services."+name+".loadbalancer.server.port"] = strconv.Itoa(config.Port)
	}
	middlewares := []string{}
	keepTLS := config.Middlewares.NoTLS == nil || !*config.Middlewares.NoTLS
	keepHTTP := config.Middlewares.NoHTTP == nil || !*config.Middlewares.NoHTTP

	// No need to have HTTPS if HTTP is disabled as well
	if keepTLS && keepHTTP {
		maps.Copy(labels, map[string]string{
			"traefik.http.middlewares.https-only.redirectscheme.permanent": "true",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":    "https",
			"traefik.http.routers." + name + `-secure.rule`:                rule,
			"traefik.http.routers." + name + `-secure.tls.certresolver`:    "defaultresolver",
			"traefik.http.routers." + name + `-secure.tls`:                 "true",
			"traefik.http.routers." + name + `-secure.entrypoints`:         "websecure",
		})
		middlewares = append(middlewares, "https-only")
	}
	if keepHTTP {
		maps.Copy(labels, map[string]string{
			"traefik.enable":                                "true",
			"traefik.http.routers." + name + `.rule`:        rule,
			"traefik.http.routers." + name + `.entrypoints`: "web",
		})
	} else {
		labels["traefik.enable"] = "false"
	}
	if config.Middlewares.Compression != nil && *config.Middlewares.Compression {
		// No need to rename compress middleware since it's same everywhere
		labels["traefik.http.middlewares.compress.compress"] = "true"
		middlewares = append(middlewares, "compress")
	}
	if config.Middlewares.AddPrefix != nil {
		middlewareName := "addPrefix-" + name
		labels["traefik.http.middlewares."+middlewareName+".addprefix"] = *config.Middlewares.AddPrefix
		middlewares = append(middlewares, middlewareName)
	}
	if config.Middlewares.StripPrefix != nil {
		middlewareName := "stripPrefix-" + name
		labels["traefik.http.middlewares."+middlewareName+".stripprefix.prefixes"] = strings.Join(*config.Middlewares.StripPrefix, ",")
		middlewares = append(middlewares, middlewareName)
	}
	if config.Middlewares.BasicAuth != nil {
		middlewareName := "basicAuth-" + name
		labels["traefik.http.middlewares."+middlewareName+".basicauth.users"] = strings.Join(*config.Middlewares.BasicAuth, ",")
		middlewares = append(middlewares, middlewareName)
	}
	if config.Middlewares.RateLimiting != nil {
		middlewareName := "ratelimit-" + name
		maps.Copy(labels, map[string]string{
			"traefik.http.middlewares." + middlewareName + ".ratelimit.average": fmt.Sprint(config.Middlewares.RateLimiting.Average),
			"traefik.http.middlewares." + middlewareName + ".ratelimit.burst":   fmt.Sprint(config.Middlewares.RateLimiting.Burst),
		})
		middlewares = append(middlewares, middlewareName)
	}
	if len(middlewares) > 0 {
		labels["traefik.http.routers."+name+`.middlewares`] = strings.Join(middlewares, ", ")
		labels["traefik.http.routers."+name+`-secure.middlewares`] = strings.Join(middlewares, ", ")
	}
	return labels
}

func makeContainerLabels(config jigtypes.DeploymentConfig) map[string]string {
	return makeDeploymentLabels(config, "container")
}

func makeDeploymentLabels(config jigtypes.DeploymentConfig, kind string) map[string]string {
	labels := map[string]string{
		"jig.name": config.Name,
	}
	labels["jig.deployment-kind"] = kind
	return labels
}

func makeVolumeMounts(config jigtypes.DeploymentConfig) ([]mount.Mount, error) {
	mounts := []mount.Mount{}
	for _, volume := range config.Volumes {
		parts := strings.SplitN(volume, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, errors.New("Invalid volume")
		}
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: parts[0],
			Target: parts[1],
		})
	}
	return mounts, nil
}

func makeRestartPolicy(config jigtypes.DeploymentConfig) (container.RestartPolicy, error) {
	if config.RestartPolicy == "" {
		return container.RestartPolicy{}, nil
	}

	if strings.Contains(config.RestartPolicy, ":") {
		parts := strings.Split(config.RestartPolicy, ":")
		retryCount, err := strconv.Atoi(parts[1])
		if err != nil {
			return container.RestartPolicy{}, errors.New("Failed to parse retry count")
		}
		return container.RestartPolicy{
			Name:              container.RestartPolicyMode(parts[0]),
			MaximumRetryCount: retryCount,
		}, nil
	}

	return container.RestartPolicy{
		Name: container.RestartPolicyMode(config.RestartPolicy),
	}, nil
}

func makeSwarmRestartPolicy(config jigtypes.DeploymentConfig) (*swarm.RestartPolicy, error) {
	if config.RestartPolicy == "" {
		return nil, nil
	}

	condition := swarm.RestartPolicyConditionAny
	switch {
	case strings.HasPrefix(config.RestartPolicy, "unless-stopped"):
		condition = swarm.RestartPolicyConditionAny
	case strings.HasPrefix(config.RestartPolicy, "always"):
		condition = swarm.RestartPolicyConditionAny
	case strings.HasPrefix(config.RestartPolicy, "on-failure"):
		condition = swarm.RestartPolicyConditionOnFailure
	case strings.HasPrefix(config.RestartPolicy, "no"):
		condition = swarm.RestartPolicyConditionNone
	}

	policy := &swarm.RestartPolicy{Condition: condition}
	if strings.Contains(config.RestartPolicy, ":") {
		parts := strings.Split(config.RestartPolicy, ":")
		retryCount, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return nil, errors.New("Failed to parse retry count")
		}
		policy.MaxAttempts = &retryCount
	}
	return policy, nil
}

func makeSwarmConstraints(config jigtypes.DeploymentConfig) ([]string, error) {
	if len(config.Placement.RequiredNodeLabels) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(config.Placement.RequiredNodeLabels))
	for key := range config.Placement.RequiredNodeLabels {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	constraints := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(config.Placement.RequiredNodeLabels[key])
		if strings.TrimSpace(key) == "" || value == "" {
			return nil, errors.New("placement.requiredNodeLabels must contain non-empty keys and values")
		}
		constraints = append(constraints, fmt.Sprintf("node.labels.%s == %s", key, value))
	}
	return constraints, nil
}

func validateSwarmConfig(config jigtypes.DeploymentConfig) error {
	if len(config.Volumes) > 0 && len(config.Placement.RequiredNodeLabels) == 0 {
		return errors.New("Swarm deployments with bind mounts require placement.requiredNodeLabels")
	}
	_, err := makeSwarmConstraints(config)
	return err
}

func untar(dst string, reader io.Reader) error {
	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dst, header.Name)
		cleanTarget := filepath.Clean(target)
		if cleanTarget != dst && !strings.HasPrefix(cleanTarget, dst+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar entry %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(cleanTarget, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(cleanTarget), 0755); err != nil {
				return err
			}
			file, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		}
	}
}

func yamlQuote(value string) string {
	return strconv.Quote(value)
}

type composeProject struct {
	Services map[string]composeProjectService `yaml:"services"`
}

type composeProjectService struct {
	Jig *jigtypes.DeploymentConfig `yaml:"x-jig"`
}

type composeManagedService struct {
	StackName   string
	ServiceName string
	DisplayName string
	Config      jigtypes.DeploymentConfig
	Envs        map[string]string
}

func mergeEnvMaps(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}

	merged := make(map[string]string, len(base)+len(override))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func mergeDeploymentConfig(base, override jigtypes.DeploymentConfig, defaultName string) jigtypes.DeploymentConfig {
	merged := base
	if override.Name != "" {
		merged.Name = override.Name
	} else {
		merged.Name = defaultName
	}
	if override.Port != 0 {
		merged.Port = override.Port
	}
	if override.RestartPolicy != "" {
		merged.RestartPolicy = override.RestartPolicy
	}
	if override.Domain != "" {
		merged.Domain = override.Domain
	}
	if override.Hostname != "" {
		merged.Hostname = override.Hostname
	}
	if override.Rule != "" {
		merged.Rule = override.Rule
	}
	if override.ComposeFile != "" {
		merged.ComposeFile = override.ComposeFile
	}
	if override.ComposeService != "" {
		merged.ComposeService = override.ComposeService
	}
	merged.Envs = mergeEnvMaps(base.Envs, override.Envs)
	if override.ExposePorts != nil {
		merged.ExposePorts = override.ExposePorts
	}
	if override.Volumes != nil {
		merged.Volumes = override.Volumes
	}
	if override.Middlewares != (jigtypes.DeploymentMiddleares{}) {
		merged.Middlewares = override.Middlewares
	}
	return merged
}

func validateComposeManagedConfig(config jigtypes.DeploymentConfig) error {
	if len(config.Volumes) > 0 {
		return errors.New("Compose deployments must configure volumes in the compose file")
	}
	if config.Port != 0 {
		return errors.New("Compose deployments must configure ports in the compose file")
	}
	if len(config.ExposePorts) > 0 {
		return errors.New("Compose deployments must configure published ports in the compose file")
	}
	if config.ComposeFile != "" || config.ComposeService != "" {
		return errors.New("Per-service compose Jig config cannot set composeFile or composeService")
	}
	return nil
}

func loadComposeProject(workdir, composeFile string) (composeProject, error) {
	output, err := os.ReadFile(filepath.Join(workdir, composeFile))
	if err != nil {
		return composeProject{}, err
	}
	var project composeProject
	if err := yaml.Unmarshal(output, &project); err != nil {
		return composeProject{}, err
	}
	return project, nil
}

func collectManagedComposeServices(project composeProject, baseConfig jigtypes.DeploymentConfig, secretDB *Secrets) ([]composeManagedService, error) {
	serviceNames := make([]string, 0, len(project.Services))
	for serviceName := range project.Services {
		serviceNames = append(serviceNames, serviceName)
	}
	slices.Sort(serviceNames)

	baseDefaults := baseConfig
	baseDefaults.Name = ""
	baseDefaults.ComposeFile = ""
	baseDefaults.ComposeService = ""
	baseDefaults.Port = 0
	baseDefaults.ExposePorts = nil
	baseDefaults.Volumes = nil

	managed := []composeManagedService{}
	seenNames := map[string]string{}
	for _, serviceName := range serviceNames {
		service := project.Services[serviceName]
		if service.Jig == nil {
			continue
		}

		displayName := serviceName
		if service.Jig.Name != "" {
			displayName = service.Jig.Name
		}
		config := mergeDeploymentConfig(baseDefaults, *service.Jig, baseConfig.Name+"-"+displayName)
		if config.Name == "" {
			config.Name = baseConfig.Name + "-" + displayName
		}
		if config.Hostname == "" {
			config.Hostname = displayName
		}
		if err := validateComposeManagedConfig(config); err != nil {
			return nil, fmt.Errorf("service %s: %w", serviceName, err)
		}

		if previousService, exists := seenNames[displayName]; exists {
			return nil, fmt.Errorf("services %s and %s both resolve to deployment name %s", previousService, serviceName, displayName)
		}
		seenNames[displayName] = serviceName

		envs, err := makeEnvMap(config.Envs, secretDB)
		if err != nil {
			return nil, fmt.Errorf("service %s: %w", serviceName, err)
		}

		managed = append(managed, composeManagedService{
			StackName:   baseConfig.Name,
			ServiceName: serviceName,
			DisplayName: displayName,
			Config:      config,
			Envs:        envs,
		})
	}

	return managed, nil
}

func legacyComposeManagedService(baseConfig jigtypes.DeploymentConfig, services []string, secretDB *Secrets) ([]composeManagedService, error) {
	if len(baseConfig.Volumes) > 0 {
		return nil, errors.New("Compose deployments must configure volumes in the compose file")
	}
	if baseConfig.Port != 0 {
		return nil, errors.New("Compose deployments must configure ports in the compose file")
	}
	if len(baseConfig.ExposePorts) > 0 {
		return nil, errors.New("Compose deployments must configure published ports in the compose file")
	}

	primaryService, err := pickComposePrimaryService(baseConfig, services)
	if err != nil {
		return nil, err
	}
	envMap, err := makeEnvMap(baseConfig.Envs, secretDB)
	if err != nil {
		return nil, err
	}

	config := baseConfig
	config.Name = baseConfig.Name + "-" + primaryService
	if config.Hostname == "" {
		config.Hostname = primaryService
	}
	config.Port = 0
	config.ExposePorts = nil
	config.Volumes = nil

	return []composeManagedService{{
		StackName:   baseConfig.Name,
		ServiceName: primaryService,
		DisplayName: primaryService,
		Config:      config,
		Envs:        envMap,
	}}, nil
}

func makeComposeOverride(managedServices []composeManagedService) string {
	var builder strings.Builder
	builder.WriteString("services:\n")
	services := append([]composeManagedService{}, managedServices...)
	slices.SortFunc(services, func(a, b composeManagedService) int {
		return strings.Compare(a.ServiceName, b.ServiceName)
	})

	for _, service := range services {
		builder.WriteString("  " + yamlQuote(service.ServiceName) + ":\n")
		if service.Config.RestartPolicy != "" {
			builder.WriteString("    restart: " + yamlQuote(service.Config.RestartPolicy) + "\n")
		}
		if len(service.Envs) > 0 {
			builder.WriteString("    environment:\n")
			keys := make([]string, 0, len(service.Envs))
			for key := range service.Envs {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			for _, key := range keys {
				builder.WriteString("      " + yamlQuote(key) + ": " + yamlQuote(service.Envs[key]) + "\n")
			}
		}

		labels := makeComposeContainerLabels(service)
		if len(labels) > 0 {
			builder.WriteString("    labels:\n")
			keys := make([]string, 0, len(labels))
			for key := range labels {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			for _, key := range keys {
				builder.WriteString("      " + yamlQuote(key) + ": " + yamlQuote(labels[key]) + "\n")
			}
		}
	}
	return builder.String()
}

func makeComposeContainerLabels(service composeManagedService) map[string]string {
	labels := makeDeploymentLabels(service.Config, "compose")
	maps.Copy(labels, makeLabels(service.Config))
	labels["jig.stack"] = service.StackName
	labels["jig.service"] = service.DisplayName
	labels["jig.display-name"] = labels["jig.stack"] + ":" + service.DisplayName
	labels["jig.name"] = service.Config.Name
	return labels
}

func runComposeCommand(workdir string, args ...string) ([]byte, error) {
	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir = workdir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func listComposeServices(workdir, composeFile string) ([]string, error) {
	output, err := runComposeCommand(workdir, "-f", composeFile, "config", "--services")
	if err != nil {
		return nil, err
	}

	services := []string{}
	for _, line := range strings.Split(string(output), "\n") {
		service := strings.TrimSpace(line)
		if service != "" {
			services = append(services, service)
		}
	}
	if len(services) == 0 {
		return nil, errors.New("docker compose did not return any services")
	}
	return services, nil
}

func pickComposePrimaryService(config jigtypes.DeploymentConfig, services []string) (string, error) {
	if config.ComposeService != "" {
		if slices.Contains(services, config.ComposeService) {
			return config.ComposeService, nil
		}
		return "", fmt.Errorf("compose service %s not found", config.ComposeService)
	}
	if slices.Contains(services, config.Name) {
		return config.Name, nil
	}
	if len(services) == 1 {
		return services[0], nil
	}
	return services[0], nil
}

func internalHostname(config jigtypes.DeploymentConfig) string {
	if config.Hostname != "" {
		return config.Hostname
	}
	return config.Name
}

func connectComposePrimaryServiceToNetwork(cli *client.Client, projectName, serviceName, alias string) error {
	containers, err := cli.ContainerList(context.Background(), container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
			filters.Arg("label", "com.docker.compose.service="+serviceName),
		),
	})
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return fmt.Errorf("compose deployment did not create any containers for service %s", serviceName)
	}

	for _, containerInfo := range containers {
		err := cli.NetworkConnect(context.Background(), "jig", containerInfo.ID, &network.EndpointSettings{
			Aliases: []string{alias},
		})
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			return err
		}
	}
	return nil
}

func connectManagedComposeServicesToNetwork(cli *client.Client, projectName string, managedServices []composeManagedService) error {
	for _, service := range managedServices {
		if err := connectComposePrimaryServiceToNetwork(cli, projectName, service.ServiceName, internalHostname(service.Config)); err != nil {
			return err
		}
	}
	return nil
}

func removeDeploymentContainers(cli *client.Client, name string) (bool, error) {
	containers, err := cli.ContainerList(context.Background(), container.ListOptions{
		All: true,
	})
	if err != nil {
		return false, err
	}

	found := false
	for _, containerInfo := range containers {
		if containerInfo.Labels["jig.name"] != name {
			continue
		}
		found = true
		if err := cli.ContainerRemove(context.Background(), containerInfo.ID, container.RemoveOptions{Force: true}); err != nil {
			return true, err
		}
	}
	return found, nil
}

func (d *DeploymentsRouter) deployCompose(w http.ResponseWriter, r *http.Request, config jigtypes.DeploymentConfig) {
	tempDir, err := os.MkdirTemp("", "jig-compose-*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tempDir)

	if err := untar(tempDir, r.Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	composePath := filepath.Join(tempDir, config.ComposeFile)
	if _, err := os.Stat(composePath); err != nil {
		http.Error(w, fmt.Sprintf("compose file %s not found in upload", config.ComposeFile), http.StatusBadRequest)
		return
	}

	project, err := loadComposeProject(tempDir, config.ComposeFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("parse compose file: %s", err.Error()), http.StatusBadRequest)
		return
	}

	services, err := listComposeServices(tempDir, config.ComposeFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	managedServices, err := collectManagedComposeServices(project, config, d.secret_db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(managedServices) == 0 {
		managedServices, err = legacyComposeManagedService(config, services, d.secret_db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	for _, service := range managedServices {
		if _, err := removeDeploymentContainers(d.cli, service.Config.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	overridePath := filepath.Join(tempDir, ".jig.compose.override.yaml")
	overrideContents := makeComposeOverride(managedServices)
	if err := os.WriteFile(overridePath, []byte(overrideContents), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	output, err := runComposeCommand(tempDir, "-p", config.Name, "-f", config.ComposeFile, "-f", filepath.Base(overridePath), "up", "-d", "--build", "--remove-orphans")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := connectManagedComposeServicesToNetwork(d.cli, config.Name, managedServices); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	if len(output) > 0 {
		w.Write(output)
	}
}

type DeploymentsRouter struct {
	cli       *client.Client
	secret_db *Secrets
	backend   deploymentBackend
}

func (d *DeploymentsRouter) usesSwarm() bool {
	return d.backend == deploymentBackendSwarm
}

func isRollbackContainer(name string, container types.Container) bool {
	rollbackName := "/" + name + "-prev"
	for _, containerName := range container.Names {
		if containerName == rollbackName {
			return true
		}
	}
	return false
}

func deploymentRepresentativeScore(name string, container types.Container) int {
	score := 0
	if !isRollbackContainer(name, container) {
		score += 4
	}
	if container.Labels["jig.primary"] == "true" {
		score += 2
	}
	if container.State == "running" {
		score++
	}
	return score
}

func deploymentHealth(state string) string {
	if state == "running" {
		return "healthy"
	}
	return "unhealthy"
}

type deploymentContainerGroup struct {
	container   types.Container
	hasRollback bool
}

type composeServiceGroup struct {
	stackName string
	services  map[string]deploymentContainerGroup
}

func betterContainer(candidate, current types.Container) bool {
	candidateScore := deploymentRepresentativeScore(candidate.Labels["jig.name"], candidate)
	currentScore := deploymentRepresentativeScore(current.Labels["jig.name"], current)
	return candidateScore > currentScore
}

func buildDeployments(containers []types.Container) []jigtypes.Deployment {
	singles := map[string]deploymentContainerGroup{}
	composeStacks := map[string]*composeServiceGroup{}

	for _, container := range containers {
		stackName := container.Labels["jig.stack"]
		serviceName := container.Labels["jig.service"]
		if stackName != "" && serviceName != "" {
			stackGroup, exists := composeStacks[stackName]
			if !exists {
				stackGroup = &composeServiceGroup{
					stackName: stackName,
					services:  map[string]deploymentContainerGroup{},
				}
				composeStacks[stackName] = stackGroup
			}
			current, exists := stackGroup.services[serviceName]
			if !exists || betterContainer(container, current.container) {
				stackGroup.services[serviceName] = deploymentContainerGroup{container: container}
			}
			continue
		}

		name, isJigDeployment := container.Labels["jig.name"]
		if !isJigDeployment {
			continue
		}
		current := singles[name]
		if isRollbackContainer(name, container) {
			current.hasRollback = true
			singles[name] = current
			continue
		}
		if current.container.ID == "" || betterContainer(container, current.container) {
			current.container = container
		}
		singles[name] = current
	}

	names := make([]string, 0, len(composeStacks)+len(singles))
	for name := range composeStacks {
		names = append(names, name)
	}
	for name := range singles {
		names = append(names, name)
	}
	slices.Sort(names)

	deployments := make([]jigtypes.Deployment, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true

		if stackGroup, ok := composeStacks[name]; ok {
			childNames := make([]string, 0, len(stackGroup.services))
			for childName := range stackGroup.services {
				childNames = append(childNames, childName)
			}
			slices.Sort(childNames)

			children := make([]jigtypes.Deployment, 0, len(childNames))
			healthy := true
			for _, childName := range childNames {
				childGroup := stackGroup.services[childName]
				child := deploymentFromContainer(childGroup.container, childName, false)
				child.Kind = "compose-service"
				child.ParentName = stackGroup.stackName
				children = append(children, child)
				if child.Status != "healthy" {
					healthy = false
				}
			}

			parent := jigtypes.Deployment{
				Name:     stackGroup.stackName,
				Kind:     "compose",
				Status:   "healthy",
				Children: children,
			}
			if !healthy {
				parent.Status = "unhealthy"
			}
			if len(children) > 0 {
				parent.ID = children[0].ID
				parent.Rule = children[0].Rule
				parent.Lifetime = children[0].Lifetime
			}
			deployments = append(deployments, parent)
			continue
		}

		group := singles[name]
		if group.container.ID == "" {
			continue
		}
		deployments = append(deployments, deploymentFromContainer(group.container, name, group.hasRollback))
	}

	return deployments
}

func deploymentFromContainer(container types.Container, name string, hasRollback bool) jigtypes.Deployment {
	return jigtypes.Deployment{
		ID:          container.ID,
		Name:        name,
		Kind:        "service",
		Rule:        container.Labels["traefik.http.routers."+container.Labels["jig.name"]+`.rule`],
		Status:      deploymentHealth(container.State),
		Lifetime:    container.Status,
		HasRollback: hasRollback,
	}
}

func deploymentDisplayName(container types.Container) string {
	if displayName := container.Labels["jig.display-name"]; displayName != "" {
		return displayName
	}
	if name := container.Labels["jig.name"]; name != "" {
		return name
	}
	if len(container.Names) > 0 {
		return strings.TrimPrefix(container.Names[0], "/")
	}
	return container.ID
}

type deploymentTargetKind string

const (
	deploymentTargetSingle       deploymentTargetKind = "single"
	deploymentTargetComposeStack deploymentTargetKind = "compose-stack"
	deploymentTargetComposeChild deploymentTargetKind = "compose-child"
)

type deploymentTarget struct {
	kind        deploymentTargetKind
	name        string
	stackName   string
	serviceName string
	containers  []types.Container
}

func listContainersByLabels(cli *client.Client, labelPairs ...string) ([]types.Container, error) {
	args := filters.NewArgs()
	for i := 0; i+1 < len(labelPairs); i += 2 {
		args.Add("label", labelPairs[i]+"="+labelPairs[i+1])
	}
	return cli.ContainerList(context.Background(), container.ListOptions{
		All:     true,
		Filters: args,
	})
}

func resolveDeploymentTarget(cli *client.Client, name string) (deploymentTarget, error) {
	if stackName, serviceName, found := strings.Cut(name, ":"); found && stackName != "" && serviceName != "" {
		containers, err := listContainersByLabels(cli, "jig.stack", stackName, "jig.service", serviceName)
		if err != nil {
			return deploymentTarget{}, err
		}
		if len(containers) == 0 {
			return deploymentTarget{}, nil
		}
		return deploymentTarget{
			kind:        deploymentTargetComposeChild,
			name:        name,
			stackName:   stackName,
			serviceName: serviceName,
			containers:  containers,
		}, nil
	}

	stackContainers, err := listContainersByLabels(cli, "jig.stack", name)
	if err != nil {
		return deploymentTarget{}, err
	}
	if len(stackContainers) > 0 {
		return deploymentTarget{
			kind:       deploymentTargetComposeStack,
			name:       name,
			stackName:  name,
			containers: stackContainers,
		}, nil
	}

	singleContainers, err := listContainersByLabels(cli, "jig.name", name)
	if err != nil {
		return deploymentTarget{}, err
	}
	if len(singleContainers) > 0 {
		return deploymentTarget{
			kind:       deploymentTargetSingle,
			name:       name,
			containers: singleContainers,
		}, nil
	}

	return deploymentTarget{}, nil
}

func pickContainerByExactName(containers []types.Container, exactName string) *types.Container {
	for i := range containers {
		for _, containerName := range containers[i].Names {
			if containerName == exactName {
				return &containers[i]
			}
		}
	}
	return nil
}
func listContainerDeployments(cli *client.Client) ([]jigtypes.Deployment, error) {
	containers, err := cli.ContainerList(context.Background(), container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	return buildDeployments(containers), nil
}

func listSwarmDeployments(cli *client.Client) ([]jigtypes.Deployment, error) {
	services, err := cli.ServiceList(context.Background(), types.ServiceListOptions{})
	if err != nil {
		return nil, err
	}

	deployments := make([]jigtypes.Deployment, 0, len(services))
	for _, service := range services {
		name, ok := service.Spec.Labels["jig.name"]
		if !ok || service.Spec.Labels["jig.deployment-kind"] != "swarm" {
			continue
		}
		status := "unknown"
		lifetime := service.CreatedAt.Format("2006-01-02 15:04:05")
		if service.UpdateStatus != nil && service.UpdateStatus.Message != "" {
			lifetime = service.UpdateStatus.Message
		}
		if service.ServiceStatus != nil {
			status = fmt.Sprintf("%d/%d running", service.ServiceStatus.RunningTasks, service.ServiceStatus.DesiredTasks)
		} else {
			tasks, err := cli.TaskList(context.Background(), types.TaskListOptions{
				Filters: filters.NewArgs(filters.Arg("service", service.ID)),
			})
			if err != nil {
				return nil, err
			}
			running := 0
			for _, task := range tasks {
				if task.Status.State == swarm.TaskStateRunning {
					running++
				}
				status = string(task.Status.State)
			}
			if len(tasks) > 1 {
				status = fmt.Sprintf("%d/%d running", running, len(tasks))
			}
		}

		deployments = append(deployments, jigtypes.Deployment{
			ID:          service.ID,
			Name:        name,
			Kind:        "swarm-service",
			Rule:        service.Spec.Labels["traefik.http.routers."+name+`-secure.rule`],
			Status:      status,
			Lifetime:    lifetime,
			HasRollback: service.PreviousSpec != nil,
			Replicas:    swarmServiceDesiredReplicas(service),
		})
	}
	return deployments, nil
}

func swarmServiceDesiredReplicas(service swarm.Service) int {
	if service.ServiceStatus != nil {
		return int(service.ServiceStatus.DesiredTasks)
	}
	if service.Spec.Mode.Replicated != nil && service.Spec.Mode.Replicated.Replicas != nil {
		return int(*service.Spec.Mode.Replicated.Replicas)
	}
	return 0
}

func findSwarmServiceByDeploymentName(cli *client.Client, name string) (*swarm.Service, error) {
	services, err := cli.ServiceList(context.Background(), types.ServiceListOptions{
		Filters: filters.NewArgs(filters.Arg("label", "jig.name="+name)),
	})
	if err != nil {
		return nil, err
	}
	for _, service := range services {
		if service.Spec.Labels["jig.deployment-kind"] == "swarm" {
			return &service, nil
		}
	}
	return nil, nil
}

func removeDeploymentServices(cli *client.Client, name string) (bool, error) {
	service, err := findSwarmServiceByDeploymentName(cli, name)
	if err != nil || service == nil {
		return false, err
	}
	return true, cli.ServiceRemove(context.Background(), service.ID)
}

func scaleSwarmDeployment(cli *client.Client, name string, replicas uint64) error {
	service, err := findSwarmServiceByDeploymentName(cli, name)
	if err != nil {
		return err
	}
	if service == nil {
		return errors.New("swarm deployment not found")
	}

	var config jigtypes.DeploymentConfig
	if configString := service.Spec.Labels["jig.config"]; configString != "" {
		if err := json.Unmarshal([]byte(configString), &config); err != nil {
			return fmt.Errorf("invalid deployment config on service: %w", err)
		}
	}
	if len(config.Placement.RequiredNodeLabels) > 0 {
		return errors.New("Scaling is not supported for deployments with placement.requiredNodeLabels")
	}

	inspected, _, err := cli.ServiceInspectWithRaw(context.Background(), service.ID, types.ServiceInspectOptions{})
	if err != nil {
		return err
	}
	if inspected.Spec.Mode.Replicated == nil {
		return errors.New("Scaling is only supported for replicated services")
	}

	spec := inspected.Spec
	spec.Mode.Replicated.Replicas = &replicas
	_, err = cli.ServiceUpdate(context.Background(), inspected.ID, inspected.Version, spec, types.ServiceUpdateOptions{})
	return err
}

func swarmNodeStats(cli *client.Client) ([]jigtypes.SwarmNodeStats, error) {
	nodes, err := cli.NodeList(context.Background(), types.NodeListOptions{})
	if err != nil {
		return nil, err
	}
	tasks, err := cli.TaskList(context.Background(), types.TaskListOptions{})
	if err != nil {
		return nil, err
	}

	taskCounts := map[string]struct{ running, total int }{}
	for _, task := range tasks {
		counts := taskCounts[task.NodeID]
		counts.total++
		if task.Status.State == swarm.TaskStateRunning {
			counts.running++
		}
		taskCounts[task.NodeID] = counts
	}

	result := make([]jigtypes.SwarmNodeStats, 0, len(nodes))
	for _, node := range nodes {
		counts := taskCounts[node.ID]
		result = append(result, jigtypes.SwarmNodeStats{
			Name:         node.Description.Hostname,
			Role:         string(node.Spec.Role),
			Availability: string(node.Spec.Availability),
			State:        string(node.Status.State),
			Address:      node.Status.Addr,
			Cpus:         node.Description.Resources.NanoCPUs / 1_000_000_000,
			MemoryBytes:  node.Description.Resources.MemoryBytes,
			RunningTasks: counts.running,
			TotalTasks:   counts.total,
		})
	}
	slices.SortFunc(result, func(a, b jigtypes.SwarmNodeStats) int {
		return strings.Compare(a.Name, b.Name)
	})
	return result, nil
}

func makeSwarmEndpointSpec(config jigtypes.DeploymentConfig) (*swarm.EndpointSpec, error) {
	if len(config.ExposePorts) == 0 {
		return nil, nil
	}

	ports := make([]swarm.PortConfig, 0, len(config.ExposePorts))
	for portProto, hostPort := range config.ExposePorts {
		target, err := strconv.ParseUint(portProto, 10, 32)
		protocol := swarm.PortConfigProtocolTCP
		if err != nil {
			targetPort, proto, splitOK := strings.Cut(portProto, "/")
			if !splitOK {
				return nil, errors.New("Invalid port format")
			}
			target, err = strconv.ParseUint(targetPort, 10, 32)
			if err != nil {
				return nil, errors.New("Invalid port format")
			}
			if strings.EqualFold(proto, "udp") {
				protocol = swarm.PortConfigProtocolUDP
			}
		}
		published, err := strconv.ParseUint(hostPort, 10, 32)
		if err != nil {
			return nil, errors.New("Invalid host port format")
		}
		ports = append(ports, swarm.PortConfig{
			Protocol:      protocol,
			TargetPort:    uint32(target),
			PublishedPort: uint32(published),
			PublishMode:   swarm.PortConfigPublishModeIngress,
		})
	}
	return &swarm.EndpointSpec{Ports: ports}, nil
}

func makeSwarmServiceSpec(config jigtypes.DeploymentConfig, image string, envs []string) (swarm.ServiceSpec, error) {
	mounts, err := makeVolumeMounts(config)
	if err != nil {
		return swarm.ServiceSpec{}, err
	}
	restartPolicy, err := makeSwarmRestartPolicy(config)
	if err != nil {
		return swarm.ServiceSpec{}, err
	}
	constraints, err := makeSwarmConstraints(config)
	if err != nil {
		return swarm.ServiceSpec{}, err
	}
	endpointSpec, err := makeSwarmEndpointSpec(config)
	if err != nil {
		return swarm.ServiceSpec{}, err
	}

	replicas := uint64(1)
	labels := makeDeploymentLabels(config, "swarm")
	maps.Copy(labels, makeRoutingLabels(config))

	return swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name:   config.Name,
			Labels: labels,
		},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image:    image,
				Env:      envs,
				Hostname: internalHostname(config),
				Labels:   labels,
				Mounts:   mounts,
			},
			Networks: []swarm.NetworkAttachmentConfig{{
				Target:  "jig",
				Aliases: []string{internalHostname(config)},
			}},
			RestartPolicy: restartPolicy,
			Placement: &swarm.Placement{
				Constraints: constraints,
			},
		},
		Mode: swarm.ServiceMode{
			Replicated: &swarm.ReplicatedService{Replicas: &replicas},
		},
		UpdateConfig: &swarm.UpdateConfig{Order: swarm.UpdateOrderStartFirst},
		EndpointSpec: endpointSpec,
	}, nil
}

func (d *DeploymentsRouter) getDeployments(w http.ResponseWriter, r *http.Request) {
	deployments, err := listContainerDeployments(d.cli)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if d.usesSwarm() {
		swarmDeployments, err := listSwarmDeployments(d.cli)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		deployments = append(deployments, swarmDeployments...)
	}

	deploymentsJson, err := json.Marshal(deployments)
	if err != nil {
		println("Failed to marshal deployments", deploymentsJson, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(deploymentsJson)
}

func (d *DeploymentsRouter) runDeploy(w http.ResponseWriter, r *http.Request) {
	cli := d.cli
	configString := r.Header.Get("x-jig-config")
	jigImageHeader := r.Header.Get("x-jig-image")
	isJigImage := jigImageHeader == "true"
	if configString == "" {
		http.Error(w, "Config not found", http.StatusBadRequest)
		return
	}
	var config jigtypes.DeploymentConfig
	if err := json.Unmarshal([]byte(configString), &config); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if config.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if config.ComposeFile != "" {
		if isJigImage {
			http.Error(w, "Compose deployments do not support prebuilt image uploads", http.StatusBadRequest)
			return
		}
		d.deployCompose(w, r, config)
		return
	}
	if d.usesSwarm() {
		if err := validateSwarmConfig(config); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	images, err := cli.ImageList(context.Background(), types.ImageListOptions{})
	if err != nil {
		log.Println("Failed to list images", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, image := range images {
		for _, tag := range image.RepoTags {
			if tag == config.Name+":latest" {
				cli.ImageTag(context.Background(), tag, config.Name+":prev")
				w.Write([]byte("Image exists, tagging as prev for rollback\n"))
			}
		}
	}

	imageRef := config.Name + ":latest"
	swarmImageRef := config.Name + ":latest"
	if d.usesSwarm() {
		swarmImageRef = fmt.Sprintf("%s:swarm-%d", config.Name, time.Now().UnixNano())
	}

	if isJigImage {
		res, err := cli.ImageLoad(context.Background(), r.Body, true)
		if err != nil {
			fmt.Println("Failed to load image for deployment", config)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if !strings.Contains(string(data), "Loaded image") || err != nil {
			fmt.Println("Failed to load image for deployment", config)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if d.usesSwarm() {
			if err := cli.ImageTag(context.Background(), imageRef, swarmImageRef); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	} else {
		buildTags := []string{imageRef}
		if d.usesSwarm() {
			buildTags = append(buildTags, swarmImageRef)
		}
		buildResponse, err := cli.ImageBuild(context.Background(), r.Body, types.ImageBuildOptions{
			Tags:        buildTags,
			Remove:      true,
			ForceRemove: true,
		})
		if err != nil {
			fmt.Println("Failed to load image for deployment", config)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		defer buildResponse.Body.Close()

		buf := bufio.NewScanner(buildResponse.Body)
		for buf.Scan() {
			jsonMessage := jsonmessage.JSONMessage{}
			json.Unmarshal(buf.Bytes(), &jsonMessage)
			jsonMessage.Display(os.Stdout, true)
			w.Write(buf.Bytes())
			w.Write([]byte{'\n'})
			w.(http.Flusher).Flush()
			if jsonMessage.Error != nil {
				buildResponse.Body.Close()
				return
			}
		}
	}

	if d.usesSwarm() {
		envs, err := makeEnvs(config.Envs, d.secret_db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		spec, err := makeSwarmServiceSpec(config, swarmImageRef, envs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		existingService, err := findSwarmServiceByDeploymentName(cli, config.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if existingService == nil {
			if _, err := cli.ServiceCreate(context.Background(), spec, types.ServiceCreateOptions{}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			if _, err := cli.ServiceUpdate(context.Background(), existingService.ID, existingService.Version, spec, types.ServiceUpdateOptions{}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if !isJigImage {
			w.Write([]byte("{\"stream\": \"\\nImage built and swarm service updated\"}\n"))
			w.(http.Flusher).Flush()
		}
		return
	}

	rollbackContainer, err := containerExistsWithName(cli, config.Name+"-prev")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rollbackContainer != nil {
		if !isJigImage {
			w.Write([]byte("\n{\"stream\": \"Rollback container exists, removing...\"}\n"))
			w.(http.Flusher).Flush()
		}
		fmt.Printf("Rollback container %s exists, stopping...\n", config.Name)
		cli.ContainerStop(context.Background(), rollbackContainer.ID, container.StopOptions{})
		fmt.Printf("Rollback container %s exists, removing\n", config.Name)
		cli.ContainerRemove(context.Background(), rollbackContainer.ID, container.RemoveOptions{})

	}

	currentContainer, err := containerExistsWithName(cli, config.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if currentContainer != nil {
		if !isJigImage {
			w.Write([]byte("\n{\"stream\": \"Current container exists, renaming...\"}\n"))
			w.(http.Flusher).Flush()
		}
		fmt.Printf("Container %s exists, using it as a rollback...\n", config.Name)
		err = cli.ContainerStop(context.Background(), currentContainer.ID, container.StopOptions{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			fmt.Printf("Failed to stop container %s: %s\n", config.Name, err.Error())
			return
		}

		fmt.Printf("Container %s exists, using it as a rollback...\n", config.Name)
		err = cli.ContainerRename(context.Background(), currentContainer.ID, config.Name+"-prev")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			fmt.Printf("Failed to rename container %s: %s\n", config.Name, err.Error())
			return
		}
	}

	exposedPorts := map[nat.Port]struct{}{}
	if config.Port != 0 {
		exposedPorts[nat.Port(fmt.Sprint(config.Port)+"/tcp")] = struct{}{}
	}

	envs, err := makeEnvs(config.Envs, d.secret_db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	restartPolicy, err := makeRestartPolicy(config)
	if err != nil {
		println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	internalHostname := internalHostname(config)

	mounts, err := makeVolumeMounts(config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Declare dHostConfig before usage
	dHostConfig := &container.HostConfig{
		RestartPolicy: restartPolicy,
		Mounts:        mounts,
	}

	if config.ExposePorts != nil {
		// config.ExposePorts is a map "<portnum>/<protocol>" => "portnum"
		for portProto, hostPort := range config.ExposePorts {
			port, err := nat.NewPort("tcp", portProto)
			if err != nil {
				// try udp if tcp fails
				port, err = nat.NewPort("udp", portProto)
				if err != nil {
					http.Error(w, "Invalid port format", http.StatusBadRequest)
					return
				}
			}
			hostPortBinding := nat.PortBinding{
				HostIP:   "0.0.0.0",
				HostPort: hostPort,
			}
			if dHostConfig.PortBindings == nil {
				dHostConfig.PortBindings = nat.PortMap{}
			}
			dHostConfig.PortBindings[port] = []nat.PortBinding{hostPortBinding}
		}
	}

	labels := makeContainerLabels(config)
	maps.Copy(labels, makeLabels(config))

	_, err = cli.ContainerCreate(context.Background(), &container.Config{
		ExposedPorts: exposedPorts,
		Env:          envs,
		Image:        config.Name + ":latest",
		Labels:       labels,
	}, dHostConfig, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			"jig": {
				Aliases: []string{internalHostname},
			},
		},
	}, &v1.Platform{}, config.Name)
	if err != nil {
		println("Failed to create container", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Printf("Container %s created\n", config.Name)

	err = cli.ContainerStart(context.Background(), config.Name, container.StartOptions{})
	if err != nil {
		println("Failed to start container", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Printf("Container %s started\n", config.Name)
	if !isJigImage {
		w.Write([]byte("{\"stream\": \"\\nImage built and container started\"}\n"))
		w.(http.Flusher).Flush()
	}
}

func (d *DeploymentsRouter) deleteDeploy(w http.ResponseWriter, r *http.Request) {

	name := r.PathValue("name")
	if d.usesSwarm() {
		found, err := removeDeploymentServices(d.cli, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if found {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	target, err := resolveDeploymentTarget(d.cli, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(target.containers) == 0 {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}
	for _, containerInfo := range target.containers {
		if err := d.cli.ContainerRemove(context.Background(), containerInfo.ID, container.RemoveOptions{Force: true}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d *DeploymentsRouter) rollbackDeployment(w http.ResponseWriter, r *http.Request) {
	cli := d.cli
	name := r.PathValue("name")
	if d.usesSwarm() {
		service, err := findSwarmServiceByDeploymentName(cli, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if service != nil {
			if service.PreviousSpec == nil {
				http.Error(w, "Rollback is not available for this swarm deployment", http.StatusBadRequest)
				return
			}
			_, err = cli.ServiceUpdate(context.Background(), service.ID, service.Version, service.Spec, types.ServiceUpdateOptions{
				Rollback: "previous",
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	target, err := resolveDeploymentTarget(cli, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(target.containers) == 0 {
		log.Printf("Original deployment not found: %s", name)
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}
	if target.kind != deploymentTargetSingle || target.containers[0].Labels["jig.deployment-kind"] == "compose" {
		http.Error(w, "Rollback is not supported for compose deployments", http.StatusBadRequest)
		return
	}
	if len(target.containers) == 0 {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	currentDeployment := pickContainerByExactName(target.containers, "/"+name)
	if currentDeployment == nil {
		currentDeployment = &target.containers[0]
	}
	rollbackTarget := pickContainerByExactName(target.containers, "/"+name+"-prev")
	if rollbackTarget == nil {
		log.Printf("Container not found for rollback: %s", name)
		http.Error(w, "Rollback targer doesn't exist", http.StatusNotFound)
		return
	}

	err = cli.ContainerStop(context.Background(), currentDeployment.ID, container.StopOptions{})
	if err != nil {
		http.Error(w, "Failed to stop container", http.StatusInternalServerError)
		return
	}
	err = cli.ContainerRename(context.Background(), currentDeployment.ID, name+"-old")
	if err != nil {
		http.Error(w, "Failed to rename current container", http.StatusInternalServerError)
		return
	}

	err = cli.ContainerRename(context.Background(), rollbackTarget.ID, name)
	if err != nil {
		http.Error(w, "Failed to rename rollback container", http.StatusInternalServerError)
		return
	}
	err = cli.ContainerStart(context.Background(), name, container.StartOptions{})
	if err != nil {
		http.Error(w, "Failed to start rollback container, trying to restart current deployment", http.StatusInternalServerError)
		cli.ContainerRename(context.Background(), currentDeployment.ID, name)
		cli.ContainerStart(context.Background(), name, container.StartOptions{})
		return
	}

	err = cli.ContainerRemove(context.Background(), currentDeployment.ID, container.RemoveOptions{})
	if err != nil {
		http.Error(w, "Failed to remove old container", http.StatusInternalServerError)
		return
	}

}

func (dr *DeploymentsRouter) getDeploymentLogs(w http.ResponseWriter, r *http.Request) {

	name := r.PathValue("name")
	if dr.usesSwarm() {
		service, err := findSwarmServiceByDeploymentName(dr.cli, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if service != nil {
			logs, err := dr.cli.ServiceLogs(context.Background(), service.ID, container.LogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Details:    true,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer logs.Close()
			w.Header().Set("Content-Type", "text/plain")
			io.Copy(w, logs)
			return
		}
	}
	target, err := resolveDeploymentTarget(dr.cli, name)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(target.containers) == 0 {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	if target.kind == deploymentTargetComposeStack {
		grouped := map[string][]types.Container{}
		serviceNames := make([]string, 0, len(target.containers))
		for _, containerInfo := range target.containers {
			serviceName := containerInfo.Labels["jig.service"]
			grouped[serviceName] = append(grouped[serviceName], containerInfo)
		}
		for serviceName := range grouped {
			serviceNames = append(serviceNames, serviceName)
		}
		slices.Sort(serviceNames)
		for _, serviceName := range serviceNames {
			fmt.Fprintf(w, "== %s:%s ==\n", target.stackName, serviceName)
			for _, containerInfo := range grouped[serviceName] {
				logs, err := dr.cli.ContainerLogs(context.Background(), containerInfo.ID, container.LogsOptions{
					ShowStdout: true,
					ShowStderr: true,
				})
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				io.Copy(w, logs)
				fmt.Fprint(w, "\n")
			}
		}
		return
	}

	for _, containerInfo := range target.containers {
		logs, err := dr.cli.ContainerLogs(context.Background(), containerInfo.ID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(target.containers) > 1 {
			fmt.Fprintf(w, "== %s ==\n", deploymentDisplayName(containerInfo))
		}
		io.Copy(w, logs)
		fmt.Fprint(w, "\n")
	}
}

func (dr *DeploymentsRouter) scaleDeployment(w http.ResponseWriter, r *http.Request) {
	if !dr.usesSwarm() {
		http.Error(w, "Scaling is only supported on swarm-backed instances", http.StatusBadRequest)
		return
	}

	name := r.PathValue("name")
	var request jigtypes.DeploymentScaleRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid scale request", http.StatusBadRequest)
		return
	}
	if request.Replicas < 1 {
		http.Error(w, "Replicas must be at least 1", http.StatusBadRequest)
		return
	}

	service, err := findSwarmServiceByDeploymentName(dr.cli, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if service == nil {
		http.Error(w, "Scaling is only supported for singular swarm service deployments", http.StatusBadRequest)
		return
	}

	if err := scaleSwarmDeployment(dr.cli, name, uint64(request.Replicas)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (dr *DeploymentsRouter) getDeploymentStats(w http.ResponseWriter, r *http.Request) {
	if dr.usesSwarm() {
		nodes, err := swarmNodeStats(dr.cli)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respondWithJson(w, http.StatusOK, jigtypes.DeploymentStatsResponse{
			Mode:  "swarm",
			Nodes: nodes,
		})
		return
	}

	containers, err := dr.cli.ContainerList(context.Background(), container.ListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{Key: "label", Value: "jig.name"}),
	})
	if err != nil {
		log.Print("Failed to list the containers")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	var allStats []jigtypes.Stats = make([]jigtypes.Stats, 0, len(containers))
	for _, container := range containers {
		stats, err := dr.cli.ContainerStatsOneShot(context.Background(), container.ID)

		if err != nil {
			log.Print("Failed to get container stats info")
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		body, err := io.ReadAll(stats.Body)
		if err != nil {
			log.Println("Somehow failed to read the body", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		defer stats.Body.Close()
		var containerStats types.StatsJSON
		if err := json.Unmarshal(body, &containerStats); err != nil {
			log.Println("Failed to unmarshal stats", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		usedMemory :=
			containerStats.MemoryStats.Usage -
				containerStats.MemoryStats.Stats["cache"]

		cpuD :=
			containerStats.CPUStats.CPUUsage.TotalUsage -
				containerStats.PreCPUStats.CPUUsage.TotalUsage

		sysCpuD :=
			containerStats.CPUStats.SystemUsage -
				containerStats.PreCPUStats.CPUUsage.TotalUsage

		cpuNum := containerStats.CPUStats.OnlineCPUs

		allStats = append(allStats, jigtypes.Stats{
			Name:             deploymentDisplayName(container),
			MemoryBytes:      math.Round((float64(usedMemory)/(1024*1024))*100) / 100,
			MemoryPercentage: math.Round((float64(usedMemory)/float64(containerStats.MemoryStats.Limit))*10000) / 100,
			CpuPercentage:    math.Round((float64(cpuD)/float64(sysCpuD))*float64(cpuNum)*10000) / 100,
		})
	}

	respondWithJson(w, http.StatusOK, jigtypes.DeploymentStatsResponse{
		Mode:  "containers",
		Stats: allStats,
	})
}

func (dr DeploymentsRouter) Router() (r chi.Router) {
	r = chi.NewRouter()
	r.Get("/", dr.getDeployments)

	r.Post("/", dr.runDeploy)

	r.Delete("/{name}", dr.deleteDeploy)

	r.Post("/{name}/rollback", dr.rollbackDeployment)

	r.Post("/{name}/scale", dr.scaleDeployment)

	r.Get("/{name}/logs", dr.getDeploymentLogs)

	r.Get("/stats", dr.getDeploymentStats)
	return r
}

func containerExistsWithName(cli *client.Client, name string) (*types.Container, error) {
	containers, err := cli.ContainerList(context.Background(), container.ListOptions{
		All: true,
	})
	if err != nil {
		return nil, err
	}
	for _, containerInfo := range containers {

		namesInclude := false
		for _, containerName := range containerInfo.Names {
			if containerName == "/"+name {
				namesInclude = true
				break
			}
		}

		if containerInfo.Labels["jig.name"] == name || namesInclude {
			return &containerInfo, nil
		}
	}
	return nil, nil
}
