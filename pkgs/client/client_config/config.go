package client_config

import (
	"encoding/json"
	"errors"
	"log"
	"os"
)

type Config struct {
	Endpoint       string `json:"endpoint"`
	Token          string `json:"token"`
	Servers        map[string]string
	SelectedServer string
}

func (c *Config) IsReadyToDeploy() (bool, error) {
	if c.Token == "" {
		return false, errors.New("no token set")
	}
	if c.Endpoint == "" {
		return false, errors.New("no endpoint set")
	}
	return true, nil
}

func InitConfig() (Config, error) {
	newConfig := Config{
		Servers:        make(map[string]string),
		Endpoint:       "",
		Token:          "",
		SelectedServer: "",
	}
	newConfig.Servers = make(map[string]string)
	return newConfig, nil
}

func (c *Config) AddServer(endpoint string, token string) {
	c.Servers[endpoint] = endpoint
	c.SelectedServer = endpoint
	c.Endpoint = endpoint
	c.Token = token
	c.Persist()
}

func (c *Config) ListServers() []string {
	var servers []string = make([]string, 0, len(c.Servers))
	for server := range c.Servers {
		servers = append(servers, server)
	}

	return servers
}

var ErrServerNotSaved = errors.New("no server selected")

func (c *Config) SelectServer(endpoint string) error {
	token, ok := c.Servers[endpoint]
	if !ok {
		return ErrServerNotSaved
	}
	c.SelectedServer = endpoint
	c.Endpoint = endpoint
	c.Token = token

	return c.Persist()
}

func (c *Config) ReadFromFile(filename string) error {
	homedir, err := os.UserHomeDir()
	if err != nil {
		log.Println("Error getting home directory")
		return nil
	}
	configJson, err := os.ReadFile(homedir + "/.jig/config.json")
	if err != nil {
		log.Println("Error reading config file: ", err)
		return nil
	}
	var config ConfigfileJson
	err = json.Unmarshal(configJson, &config)
	if err != nil {
		log.Println("Error unmarshalling config file: ", err)
		return errors.New("error parsing config file")
	}

	if config.LastUsedServer != "" {
		c.SelectedServer = config.LastUsedServer
	}

	for _, server := range config.Servers {
		c.Servers[server.Endpoint] = server.Token
	}

	return nil
}

func (c *Config) Persist() error {
	var persistableConfig ConfigfileJson
	if c.Servers != nil {
		for endpoint, token := range c.Servers {
			persistableConfig.Servers = append(persistableConfig.Servers, ServerConfig{Endpoint: endpoint, Token: token})
		}
	}
	persistableConfig.LastUsedServer = c.SelectedServer

	configJson, err := json.Marshal(persistableConfig)
	if err != nil {
		return err
	}
	homedir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	os.Mkdir(homedir+"/.jig", 0755)
	err = os.WriteFile(homedir+"/.jig/config.json", configJson, 0644)
	if err != nil {
		return err
	}
	return nil

}

type ServerConfig struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

type ConfigfileJson struct {
	Servers        []ServerConfig `json:"servers"`
	LastUsedServer string         `json:"lastUsedServer"`
}
