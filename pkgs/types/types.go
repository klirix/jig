package jigtypes

type DeploymentConfig struct {
	Name           string               `json:"name" yaml:"name"`
	Port           int                  `json:"port" yaml:"port"`
	RestartPolicy  string               `json:"restartPolicy" yaml:"restartPolicy"`
	Domain         string               `json:"domain" yaml:"domain"`
	Hostname       string               `json:"hostname" yaml:"hostname"`
	Rule           string               `json:"rule" yaml:"rule"`
	ComposeFile    string               `json:"composeFile" yaml:"composeFile"`
	ComposeService string               `json:"composeService" yaml:"composeService"`
	Placement      DeploymentPlacement  `json:"placement" yaml:"placement"`
	Envs           map[string]string    `json:"envs" yaml:"envs"`
	ExposePorts    map[string]string    `json:"exposePorts" yaml:"exposePorts"`
	Volumes        []string             `json:"volumes" yaml:"volumes"`
	Middlewares    DeploymentMiddleares `json:"middlewares" yaml:"middlewares"`
}

type DeploymentPlacement struct {
	RequiredNodeLabels map[string]string `json:"requiredNodeLabels" yaml:"requiredNodeLabels"`
}

type DeploymentMiddleares struct {
	NoTLS        *bool                `json:"noTLS" yaml:"noTLS"`
	NoHTTP       *bool                `json:"noHTTP" yaml:"noHTTP"`
	RateLimiting *RateLimitMiddleware `json:"rateLimiting" yaml:"rateLimiting"`
	StripPrefix  *[]string            `json:"stripPrefix" yaml:"stripPrefix"`
	AddPrefix    *string              `json:"addPrefix" yaml:"addPrefix"`
	Compression  *bool                `json:"compression" yaml:"compression"`
	BasicAuth    *[]string            `json:"basicAuth" yaml:"basicAuth"`
}

type RateLimitMiddleware struct {
	Average int `json:"average" yaml:"average"`
	Burst   int `json:"burst" yaml:"burst"`
}

type Deployment struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Rule        string       `json:"rule"`
	Status      string       `json:"status"`
	Lifetime    string       `json:"lifetime"`
	HasRollback bool         `json:"hasRollback"`
	Children    []Deployment `json:"children,omitempty"`
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

type Stats struct {
	Name             string  `json:"name"`
	CpuPercentage    float64 `json:"cpuPercentage"`
	MemoryPercentage float64 `json:"memoryPercentage"`
	MemoryBytes      float64 `json:"memoryBytes"`
}

type TokenListResponse struct {
	TokenNames []string `json:"tokens"`
}

type TokenCreateRequest struct {
	Name string `json:"name"`
}
type TokenCreateResponse struct {
	Name  string `json:"name"`
	Token string `json:"token"`
}
