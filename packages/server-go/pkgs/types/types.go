package types

type DeploymentConfig struct {
	Name          string            `json:"name"`
	Port          int               `json:"port"`
	RestartPolicy string            `json:"restartPolicy"`
	Domain        string            `json:"domain"`
	Rule          string            `json:"rule"`
	Envs          map[string]string `json:"envs"`
}

type Deployment struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Rule   string `json:"rule"`
	Status string `json:"status"`
}

type NewSecretBody struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SecretList struct {
	Secrets []string `json:"secrets"`
}

type SecretInspect struct {
	Value string `json:"value"`
}
