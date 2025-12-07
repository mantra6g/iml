from http import HTTPStatus
from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse
from typing import List
from ipaddress import IPv6Network
from pydantic import BaseModel
import uvicorn

app = FastAPI()

assignments = []

class AssignmentPayload(BaseModel):
	subfunction_id: int
	sid: IPv6Network

class SidAssignment:
	def __init__(self, vnf_id, group_id, subfunction_id, sid):
		self.vnf_id = vnf_id
		self.group_id = group_id
		self.subfunction_id = subfunction_id
		self.sid = sid
	
	def to_dict(self):
		return {
			"vnf_id": self.vnf_id,
			"group_id": self.group_id,
			"subfunction_id": self.subfunction_id,
			"sid": str(self.sid)
		}

# ------------------------------------------------------------------------
# ASSIGNMENT CREATION
# ------------------------------------------------------------------------

@app.post('/api/v1/p4controller/assignments/{vnf_id}/{group_id}')
def receive_assignment(vnf_id: str, group_id: str, assignments_payload: List[AssignmentPayload]):
	for assignment in assignments_payload:	
		assignment = SidAssignment(
			vnf_id=vnf_id,
			group_id=group_id,
			subfunction_id=assignment.subfunction_id,
			sid=assignment.sid
		)

		# Store assignment
		assignments.append(assignment)	

	return JSONResponse(
		content={"status": "assignment received", "assignment": assignment.to_dict()}, 
		status_code=HTTPStatus.CREATED
	)

# ------------------------------------------------------------------------
# ASSIGNMENT RETRIEVAL
# ------------------------------------------------------------------------

@app.get('/api/v1/p4controller/assignments/{vnf_id}')
def get_all_vnf_assignments(vnf_id: str):
	vnf_assignments = []
	for assignment in assignments:
		if assignment.vnf_id == vnf_id:
			vnf_assignments.append(assignment.to_dict())
	
	return JSONResponse(
		content=vnf_assignments, 
		status_code=HTTPStatus.OK
	)

# ------------------------------------------------------------------------
# ASSIGNMENT DELETION
# ------------------------------------------------------------------------

@app.delete('/api/v1/p4controller/assignments/{vnf_id}')
def delete_all_vnf_assignments(vnf_id: str):
	global assignments
	assignments = [a for a in assignments if a.vnf_id != vnf_id]
	return JSONResponse(
		content={"status": f"all assignments for VNF ID {vnf_id} deleted"}, 
		status_code=HTTPStatus.NO_CONTENT
	)

@app.delete('/api/v1/p4controller/assignments/{vnf_id}/{group_id}')
def delete_vnf_group_assignments(vnf_id: str, group_id: str):
	global assignments
	assignments = [a for a in assignments if not (a.vnf_id == vnf_id and a.group_id == group_id)]
	return JSONResponse(
		content={"status": f"all assignments for VNF ID {vnf_id} and Group ID {group_id} deleted"}, 
		status_code=HTTPStatus.NO_CONTENT
	)


if __name__ == '__main__':
	uvicorn.run(app, host='0.0.0.0', port=5000)