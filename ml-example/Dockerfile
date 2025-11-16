FROM rayproject/ray:2.46.0

WORKDIR /app
COPY train.py /app/train.py
COPY serve_app.py /app/serve_app.py
RUN pip install mlflow scikit-learn joblib

ENV PYTHONPATH=/app