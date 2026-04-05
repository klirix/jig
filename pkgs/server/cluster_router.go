package main

import (
	"context"
	"fmt"
	"net/http"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"
)

type ClusterRouter struct {
	cli     *client.Client
	backend deploymentBackend
}

func swarmManagerAddress(info swarm.Info) string {
	if len(info.RemoteManagers) > 0 && info.RemoteManagers[0].Addr != "" {
		return info.RemoteManagers[0].Addr
	}
	if info.NodeAddr != "" {
		return info.NodeAddr + ":2377"
	}
	return ""
}

func (cr ClusterRouter) getStatus(w http.ResponseWriter, r *http.Request) {
	response := jigtypes.ClusterStatusResponse{
		Backend: string(cr.backend),
	}
	if cr.backend == deploymentBackendSwarm {
		info, err := cr.cli.Info(context.Background())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		nodes, err := swarmNodeStats(cr.cli)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Nodes = nodes
		response.ManagerAddress = swarmManagerAddress(info.Swarm)
	}
	respondWithJson(w, http.StatusOK, response)
}

func (cr ClusterRouter) getJoinToken(w http.ResponseWriter, r *http.Request) {
	if cr.backend != deploymentBackendSwarm {
		http.Error(w, "Join tokens are only available on swarm-backed instances", http.StatusBadRequest)
		return
	}

	role := r.PathValue("role")
	if role != "worker" && role != "manager" {
		http.Error(w, "Role must be worker or manager", http.StatusBadRequest)
		return
	}

	info, err := cr.cli.Info(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cluster, err := cr.cli.SwarmInspect(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	token := cluster.JoinTokens.Worker
	if role == "manager" {
		token = cluster.JoinTokens.Manager
	}
	managerAddress := swarmManagerAddress(info.Swarm)
	if managerAddress == "" {
		http.Error(w, "Could not determine swarm manager address", http.StatusInternalServerError)
		return
	}

	respondWithJson(w, http.StatusOK, jigtypes.ClusterJoinTokenResponse{
		Role:           role,
		Token:          token,
		Command:        fmt.Sprintf("docker swarm join --token %s %s", token, managerAddress),
		ManagerAddress: managerAddress,
	})
}

func (cr ClusterRouter) Router() chi.Router {
	r := chi.NewRouter()
	r.Get("/status", cr.getStatus)
	r.Get("/join-token/{role}", cr.getJoinToken)
	return r
}
