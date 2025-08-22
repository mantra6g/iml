from http import HTTPStatus
from flask import Blueprint

apps_bp = Blueprint("applications", __name__, url_prefix="/api/v1/apps")


@apps_bp.route("/", methods=["POST"])
def create_application():
	return "Application created", HTTPStatus.CREATED

@apps_bp.route("/<int:id>", methods=["DELETE"])
def delete_application(id: int):
	return "Application deleted", HTTPStatus.NO_CONTENT