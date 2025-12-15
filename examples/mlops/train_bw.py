import pandas as pd
from prophet import Prophet
import mlflow
import mlflow.pyfunc
import ray
from ray.train import ScalingConfig, Checkpoint, get_dataset_shard, RunConfig
from ray.train.data_parallel_trainer import DataParallelTrainer
import os
import pickle

MLFLOW_TRACKING_URI = "http://fns01.lab.uvalight.net:5000"
mlflow.set_tracking_uri(MLFLOW_TRACKING_URI)
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
	train_iter = get_dataset_shard("train")
	test_iter = get_dataset_shard("test")

	train_dfs = [batch for batch in train_iter.iter_batches(batch_format="pandas")]
	train_df = pd.concat(train_dfs, ignore_index=True)

	test_dfs = [batch for batch in test_iter.iter_batches(batch_format="pandas")]
	test_df = pd.concat(test_dfs, ignore_index=True)

	test_X = test_df[['ds']]
	test_y = test_df['y']

	model = Prophet()
	model.fit(train_df)

	# Evaluate on test set
	forecast = model.predict(test_X)
	mse = ((forecast['yhat'] - test_y) ** 2).mean()

	# Save model to a checkpoint directory
	os.makedirs("checkpoint", exist_ok=True)
	with open("checkpoint/model.pkl", "wb") as f:
		pickle.dump(model, f)

	ray.train.report(
		{"mse": mse}, 
		checkpoint=Checkpoint.from_directory("checkpoint")
	)

if __name__ == "__main__":
	ray.init()

	# Read full CSV once (on driver)
	df = pd.read_csv("/app/mawi.csv")
	df.rename(columns={"date": "ds", "bandwidth": "y"}, inplace=True)

	# Shuffle and sample 80% for the first dataset
	df_train = df.sample(frac=0.8, random_state=42)

	# Use the remaining 20% for the second dataset
	df_test = df.drop(df_train.index)

	# Create a Ray Dataset from the pandas DataFrame
	train_dataset = ray.data.from_pandas(df_train)
	test_dataset = ray.data.from_pandas(df_test)

	trainer = DataParallelTrainer(
		train_loop_per_worker=train_prophet,
		datasets={"train": train_dataset, "test": test_dataset},
		scaling_config=ScalingConfig(
			num_workers=1,
			use_gpu=False
		),
		run_config=RunConfig(
			storage_path="/mnt/bw-forecast-data/results"
		)
	)
	result = trainer.fit()
	best_checkpoint = result.get_best_checkpoint("mse", mode="min")

	# Extract the trained model from the checkpoint
	with best_checkpoint.as_directory() as checkpoint_dir:
		import pickle
		with open(f"{checkpoint_dir}/model.pkl", "rb") as f:
			prophet_model = pickle.load(f)  # This is the actual Prophet object

	import pickle
	with open("prophet_model.pkl", "wb") as f:
		pickle.dump(prophet_model, f)
	
	with mlflow.start_run():
		mlflow.pyfunc.log_model(
			name="prophet_model",
			python_model=ProphetWrapper(),
			artifacts={"model_path": "prophet_model.pkl"}
		)

	print("Model trained and logged to MLflow!")
