package jigtypes

type DeploymentConfig struct {
	Name          string               `json:"name"`
	Port          int                  `json:"port"`
	RestartPolicy string               `json:"restartPolicy"`
	Domain        string               `json:"domain"`
	Hostname      string               `json:"hostname"`
	Rule          string               `json:"rule"`
	Envs          map[string]string    `json:"envs"`
	Volumes       []string             `json:"volumes"`
	Middlewares   DeploymentMiddleares `json:"middlewares"`
}

type DeploymentMiddleares struct {
	NoTLS        *bool                `json:"noTLS"`
	NoHTTP       *bool                `json:"noHTTP"`
	RateLimiting *RateLimitMiddleware `json:"rateLimiting"`
	StripPrefix  *[]string            `json:"stripPrefix"`
	AddPrefix    *string              `json:"addPrefix"`
	Compression  *bool                `json:"compression"`
	BasicAuth    *[]string            `json:"basicAuth"`
}

type RateLimitMiddleware struct {
	Average int `json:"average"`
	Burst   int `json:"burst"`
}

type Deployment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Rule     string `json:"rule"`
	Status   string `json:"status"`
	Lifetime string `json:"lifetime"`
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
