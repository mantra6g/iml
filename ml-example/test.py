from mlflow.tracking import MlflowClient

client = MlflowClient("http://fns01.lab.uvalight.net:5000")
iris_model = client.get_registered_model("iris_model")
print(iris_model)

models = client.search_registered_models()

for m in models:
    print(m.name)