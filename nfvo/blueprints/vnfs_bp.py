from http import HTTPStatus
from flask import Blueprint

network_functions_bp = Blueprint("network_functions", __name__, url_prefix="/api/v1/vnfs")

@network_functions_bp.route("/", methods=["POST"])
def create_network_function():
	return "Network function created", HTTPStatus.CREATED

@network_functions_bp.route("/<int:id>", methods=["DELETE"])
def delete_network_function(id: int):
	return "Network function deleted", HTTPStatus.NO_CONTENT