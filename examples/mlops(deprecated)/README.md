# MLOps Workflow Example

This example showcases the **MLOps capabilities** of each cluster when working in **SMO (Service Management and Orchestration) mode**. It demonstrates how to seamlessly integrate machine learning model training, versioning, and serving within a Kubernetes-based IML cluster using MLflow and Ray.

## Overview

In SMO mode, the IML enables a complete MLOps pipeline where:
- **Model Training**: Distributed training of machine learning models using Ray, with automatic scaling and resource management
- **Model Registry**: Centralized model versioning and lifecycle management via MLflow
- **Model Serving**: Production-ready model serving infrastructure with built-in replication and load balancing

This example implements a **bandwidth forecasting system** that predicts network bandwidth using time-series forecasting (Prophet algorithm) on MAWI (Measurement and Analysis on the WIDE Internet) dataset.

## Architecture

The MLOps workflow consists of three main components:

### 1. Training Pipeline (`train_bw.py`)
- Uses **Ray Train** for distributed training across cluster nodes
- Trains Prophet models on historical bandwidth data from the MAWI dataset
- Logs models, metrics, and artifacts to **MLflow tracking server**
- Automatically registers trained models in the MLflow Model Registry

### 2. Model Registry
- Central MLflow instance (deployed externally or within the cluster)
- Manages model versions and lifecycle stages (Development, Staging, Production)
- Enables model reproducibility and auditing

### 3. Serving Infrastructure (`serve_bw.py`)
- Uses **Ray Serve** for low-latency model inference
- Loads production models directly from MLflow Model Registry
- Provides REST API endpoint for bandwidth forecasting predictions
- Automatically scales replicas based on demand

## Prerequisites

- Kubernetes cluster with IML installed
- MLflow tracking server accessible from the cluster
- Ray cluster setup (handled by Kubernetes deployments)
- Network connectivity between cluster nodes

## Deployment

### 1. Training Job

Deploy the training pipeline:
```bash
kubectl apply -f k8s/train_bw.yaml
```

This will:
- Create a Ray job that trains the bandwidth forecasting model
- Log all training metrics and artifacts to MLflow
- Register the trained model in the MLflow Model Registry

Monitor training progress:
```bash
kubectl logs -f <training-pod-name>
```

### 2. Model Serving

Once training completes and the model is in Production stage, deploy the serving application:
```bash
kubectl apply -f k8s/serve_bw.yaml
```

This will:
- Deploy Ray Serve with the BandwidthForecastingModel
- Expose REST API endpoints for predictions
- Automatically manage model loading from MLflow Registry

## Usage

### Query the Bandwidth Forecasting Service

```bash
# Port-forward to the Ray Serve service
kubectl port-forward svc/bw-forecast-serve-svc 8000:8000

# Make a prediction request
curl \
  -X POST \
  -H 'Content-Type: application/json' \
  http://localhost:8000/ \
  -d '[{"ds": "2025-01-01 00:06:00"}]'
```

## Key Benefits in SMO Mode

- **Automated Lifecycle Management**: SMO orchestrates both application deployment and network service configuration
- **Resource Optimization**: IML selects optimal cluster resources for training and serving workloads
- **Network Intelligence**: Models can be leveraged by network functions to make intelligent routing decisions
- **Multi-Cluster Support**: MLOps capabilities work consistently across all clusters in your infrastructure
- **Centralized Model Management**: Single source of truth for all production models via MLflow

## Integration with IML Network Functions

The trained bandwidth forecasting model can be integrated into network functions to:
- Predict congestion and trigger traffic rerouting
- Enable dynamic service chain adaptation
- Support intelligent load balancing decisions
- Drive SLA compliance optimization

## Data

The example uses the **MAWI dataset** (`mawi.csv`), containing real network bandwidth measurements. You can replace this with your own dataset for different forecasting tasks.

## Configuration

Update the following in the training and serving scripts to match your environment:

```python
MLFLOW_TRACKING_URI = "http://<your-mlflow-server>:5000"
mlflow.set_experiment("<your-experiment-name>")
MODEL_NAME = "<your-model-name>"
MODEL_STAGE = "Production"
```

