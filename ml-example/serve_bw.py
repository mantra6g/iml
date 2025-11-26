import pandas as pd
import mlflow
from mlflow.tracking import MlflowClient
import ray
from ray import serve

# MLflow tracking server
MLFLOW_TRACKING_URI = "http://fns01.lab.uvalight.net:5000"
mlflow.set_tracking_uri(MLFLOW_TRACKING_URI)

# Model registry parameters
MODEL_NAME = "bandwidth_forecasting_model"
MODEL_STAGE = "Production" # or specify a version manually

def load_model_from_registry():
    """
    Load model from MLflow Model Registry.
    """
    client = MlflowClient()
    # Get latest model version in given stage
    mv = client.get_latest_versions(MODEL_NAME, stages=[MODEL_STAGE])[0]
    model_uri = f"models:/{MODEL_NAME}/{mv.version}"

    print(f"Loading model from MLflow Registry: {model_uri}")
    model = mlflow.pyfunc.load_model(model_uri)
    return model


@serve.deployment(num_replicas=2, max_replicas_per_node=1)
class BandwidthForecastingModel:
    def __init__(self):
        self.model = load_model_from_registry()

    async def __call__(self, request):
        data = await request.json()
        df = pd.DataFrame(data)
        forecast = self.model.predict(df)
        return forecast.to_dict(orient="records")

app = BandwidthForecastingModel.bind()
