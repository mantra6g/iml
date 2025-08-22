from http import HTTPStatus
from flask import Blueprint

service_chains_bp = Blueprint("service_chains", __name__, url_prefix="/api/v1/chains")

@service_chains_bp.route("/", methods=["POST"])
def create_service_chain():
	return "Service chain created", HTTPStatus.CREATED

@service_chains_bp.route("/<int:id>", methods=["DELETE"])
def delete_service_chain(id: int):
	return "Service chain deleted", HTTPStatus.NO_CONTENT