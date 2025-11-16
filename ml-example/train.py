import mlflow
import ray
from sklearn.datasets import load_iris
from sklearn.ensemble import RandomForestClassifier
import joblib

# Point MLflow to the internal MLflow server on Kubernetes
MLFLOW_TRACKING_URI = "http://fns01.lab.uvalight.net:5000"
mlflow.set_tracking_uri(MLFLOW_TRACKING_URI)
mlflow.set_experiment("iris-training")

def train_model():
	data = load_iris()
	X, y = data.data, data.target

	with mlflow.start_run():
		model = RandomForestClassifier(n_estimators=150)
		model.fit(X, y)

		# Log metric
		acc = model.score(X, y)
		mlflow.log_metric("train_accuracy", acc)

		# Log artifact
		joblib.dump(model, "model.joblib")
		mlflow.log_artifact("model.joblib")

		# Log model in MLflow model registry format
		mlflow.sklearn.log_model(model, artifact_path="model")

if __name__ == "__main__":
	ray.init()
	train_model()
