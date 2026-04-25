FROM rayproject/ray:2.46.0

WORKDIR /app

COPY train_bw.py /app/train.py
COPY serve_bw.py /app/serve_app.py
COPY mawi.csv /app/mawi.csv

RUN pip install mlflow pandas prophet ray[serve,train,data]

ENV PYTHONPATH=/app