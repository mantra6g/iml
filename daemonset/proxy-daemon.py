# Proxy listens on port 7623 for requests from the CNIs
# and then forwards them to the IML using the service url.

from flask import Flask, request
import requests

app = Flask(__name__)

IML_BASE = "http://iml-nfvo.desire6g-system.svc.cluster.local:5000"

@app.route("/iml/cni/register", methods=["POST"])
def register():
    try:
        resp = requests.post(f"{IML_BASE}/iml/cni/register", json=request.get_json(), timeout=5)
        return (resp.text, resp.status_code)
    except Exception as e:
        return (f"Proxy error: {str(e)}", 500)

@app.route("/iml/cni/teardown", methods=["POST"])
def teardown():
    try:
        resp = requests.post(f"{IML_BASE}/iml/cni/teardown", json=request.get_json(), timeout=5)
        return (resp.text, resp.status_code)
    except Exception as e:
        return (f"Proxy error: {str(e)}", 500)

if __name__ == "__main__":
    app.run(host="127.0.0.1", port=7623, debug=False)