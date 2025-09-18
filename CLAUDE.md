# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Shibuya is a Kubernetes-based scheduler for load generators (primarily JMeter) that can deploy executors in a Kubernetes cluster and generate real-time test results. It provides higher scalability than distributed JMeter and offers real-time metrics through Prometheus and Grafana integration.

## Architecture

Shibuya consists of several key components:

- **API Server** (`shibuya/api/main.go`): REST API handling projects, collections, plans, and execution management
- **Controller** (`shibuya/controller/`): Core scheduler that manages engine deployment and lifecycle
- **Engine/Executor**: Load generating pods (JMeter + Agent) deployed in Kubernetes
- **Agent** (`shibuya/engines/jmeter/shibuya-agent.go`): Sidecar process that communicates with controller and manages JMeter
- **Storage backends**: Nexus, GCP Bucket, or local storage for test plans and artifacts
- **UI Handler** (`shibuya/ui/handler.go`): Web interface serving static files and templates

### Key Models
- **Project**: Top-level organization unit with ownership (LDAP-based)
- **Collection**: Container for multiple test plans that can be executed simultaneously
- **Plan**: Individual test configuration with associated files
- **ExecutionPlan**: Links plans to collections with engine count and concurrency settings

## Development Commands

### Local Development Setup
```bash
make                    # Create kind cluster and deploy all components
make expose            # Port-forward services (Shibuya on :8080, Grafana on :3000)
make clean             # Destroy local cluster
```

### Building Components
```bash
make shibuya           # Build and deploy API server and controller changes
make local_api         # Build API server only
make local_controller  # Build controller only
make jmeter           # Build JMeter engine image
```

### Testing & Monitoring
- Access Shibuya UI: http://localhost:8080
- Access Grafana: http://localhost:3000
- Prometheus metrics: http://localhost:8080/metrics

## Distributed Mode

Shibuya supports distributed mode by splitting into separate API server and controller processes:
- Enable with `runtime.distributed_mode: true` in config
- API server: handles REST endpoints and UI
- Controller: manages engine scheduling and monitoring

## Code Organization

```
shibuya/
├── api/              # REST API handlers
├── auth/             # LDAP and session management
├── config/           # Configuration management
├── controller/       # Core scheduling and engine management
├── engines/          # Engine implementations (JMeter agent, metrics)
├── model/            # Database models and business logic
├── object_storage/   # Storage abstraction (Nexus/GCP/Local)
├── scheduler/        # Kubernetes and CloudRun schedulers
├── ui/               # Web UI handlers
└── utils/            # Common utilities
```

## Authentication & Authorization
- LDAP-based authentication (can be disabled for local development)
- Project ownership based on LDAP group membership
- For local development without auth, use `shibuya` as the owner

## Key Configuration Files
- `config_env.json`: Main configuration (DB, Kubernetes, storage, etc.)
- `shibuya-gcp.json`: GCP credentials for node scaling
- Database: MariaDB v10.0.23 compatible

## Testing Notes
- Local setup uses in-cluster authentication
- Production requires proper kubeconfig and service accounts
- Resource limits enforced via `MaxEnginesInCollection` setting
