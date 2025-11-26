import pandas as pd
from prophet import Prophet
import mlflow
import mlflow.pyfunc
import ray
from ray.train import Trainer

mlflow.set_tracking_uri("http://localhost:5000")
mlflow.set_experiment("prophet_mawi_bandwidth_forecasting")

# Wrapper so MLflow can log Prophet models
class ProphetWrapper(mlflow.pyfunc.PythonModel):
	def load_context(self, context):
		import pickle
		with open(context.artifacts['model_path'], 'rb') as f:
			self.model = pickle.load(f)

	def predict(self, context, model_input):
		return self.model.predict(model_input)

# Distributed training function
def train_prophet(config):
	df = pd.read_csv("mawi.csv")
	df.rename(columns={"date": "ds", "bandwidth": "y"}, inplace=True)

	model = Prophet()
	model.fit(df)

	return model

if __name__ == "__main__":
	ray.init()

	trainer = Trainer(backend="gloo", num_workers=1)
	result = trainer.run(train_prophet, config={})
	model = result

	import pickle
	with open("prophet_model.pkl", "wb") as f:
		pickle.dump(model, f)

	with mlflow.start_run():
		mlflow.pyfunc.log_model(
			artifact_path="prophet_model",
			python_model=ProphetWrapper(),
			artifacts={"model_path": "prophet_model.pkl"}
		)

	print("Model trained and logged to MLflow!")
