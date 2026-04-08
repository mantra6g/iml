# IML - Infrastructure Management Layer

Welcome to the IML documentation!

## What is IML?

**IML (Infrastructure Management Layer)** is a local Network Function Virtualization (NFV) orchestrator designed for Kubernetes environments. It enables you to:

- **Deploy applications** and register them as traffic endpoints in your cluster
- **Deploy network functions** that inspect, transform, or route traffic
- **Create service chains** that define how traffic flows between applications through multiple network functions
- **Configure the data plane** automatically so traffic follows your specified paths

IML works seamlessly with Kubernetes and provides both **standalone workflows** (where you define resources directly) and **SMO-integrated workflows** (where a higher-level orchestrator like Oakestra manages deployment).

---

## Key Features

- **Kubernetes-Native**: Fully integrated with Kubernetes using Custom Resource Definitions (CRDs)
- **Service Chaining**: Define complex traffic paths through multiple network functions
- **Flexible Deployment**: Supports both standalone and orchestrator-integrated modes
- **Local Optimization**: Makes intelligent scheduling decisions for network function placement
- **Easy Development**: Fully containerized with support for local testing via kind

---

## Getting Started

### New to IML?

Start here to understand the basics:

1. **[Project Overview](project-overview.md)** - Learn core concepts like Applications, Network Functions, and Service Chains
2. **[Quick Start Guide](getting-started/installation.md)** - Set up IML and deploy your first service chain in minutes

### Contributing to IML

Interested in developing IML? Check out:

- **[Contributing Guide](contributing/contributing.md)** - Learn about project structure, development workflow, and how to build/test locally
- **[Architecture Overview](architecture/overview.md)** - Deep dive into how IML components work together

---

## Documentation Sections

### 📚 Core Documentation

- **[Project Overview](project-overview.md)** - High-level introduction to IML concepts and workflows
- **[Getting Started](getting-started/installation.md)** - Installation and initial setup guide
- **[Architecture](architecture/overview.md)** - System design, components, and how they interact
- **[API Reference](api/overview.md)** - Complete reference for custom resources and APIs

### 🛠️ Development & Contributing

- **[Contributing Guide](contributing/contributing.md)** - How to contribute, project structure, development workflow
- **[Architecture Details](architecture/overview.md)** - In-depth technical documentation


## Need Help?

- **Questions about IML?** Check the [Project Overview](project-overview.md)
- **Installing IML?** Follow the [Installation Guide](getting-started/installation.md)
- **Want to contribute?** Read the [Contributing Guide](contributing/contributing.md)
- **Having issues?** Reach out to tomas@tomasagata.dev
