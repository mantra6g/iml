import mlflow
import ray
from sklearn.datasets import load_iris
from sklearn.ensemble import RandomForestClassifier
from sklearn.model_selection import train_test_split
import joblib

# Point MLflow to the internal MLflow server on Kubernetes
MLFLOW_TRACKING_URI = "http://fns01.lab.uvalight.net:5000"
mlflow.set_tracking_uri(MLFLOW_TRACKING_URI)
mlflow.set_experiment("iris-training")

def train_model():
	data = load_iris()
	X_train, X_test, y_train, y_test = train_test_split(data.data, data.target, test_size=0.2, random_state=42)

	with mlflow.start_run():
		model = RandomForestClassifier(n_estimators=150)
		model.fit(X_train, y_train)
		
		# Log metric
		acc = model.score(X_train, y_train)
		mlflow.log_metric("train_accuracy", acc)

		# Log artifact
		joblib.dump(model, "model.joblib")
		mlflow.log_artifact("model.joblib")

		# Log model in MLflow model registry format
		mlflow.sklearn.log_model(model, artifact_path="model")

if __name__ == "__main__":
	ray.init()
	train_model()
