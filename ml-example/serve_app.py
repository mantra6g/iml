import mlflow
import ray
from ray import serve

MLFLOW_TRACKING_URI = "http://fns01.lab.uvalight.net:5000"
mlflow.set_tracking_uri(MLFLOW_TRACKING_URI)

# Load the latest production model from MLflow registry
MODEL_URI = "models:/iris_model@production"

@serve.deployment(num_replicas=2)
class IrisModel:
    def __init__(self):
        self.model = mlflow.pyfunc.load_model(MODEL_URI)

    async def __call__(self, request):
        data = await request.json()
        return self.model.predict([data]).tolist()

app = IrisModel.bind()
