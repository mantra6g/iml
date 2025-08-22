from pymongo import MongoClient

MONGO_URL  = "nfvo_mongodb.desire6g-system.svc.cluster.local"
MONGO_PORT = 27017
DB_ADDR = f'mongodb://{MONGO_URL}:{MONGO_PORT}/nfvo_db'

db = MongoClient(DB_ADDR)
