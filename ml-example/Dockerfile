FROM rayproject/ray:2.46.0

WORKDIR /app

COPY train_bw.py /app/train.py
COPY serve_bw.py /app/serve_app.py
COPY mawi.csv /app/mawi.csv
COPY requirements.txt /app/requirements.txt

RUN pip install --no-cache-dir -r requirements.txt

ENV PYTHONPATH=/app