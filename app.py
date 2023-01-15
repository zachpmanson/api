# app.py
from flask import Flask, request, jsonify
from flask_limiter import Limiter
from flask_limiter.util import get_remote_address
from flask_cors import CORS

import json
from datetime import date

app = Flask(__name__)
app.config.from_pyfile('config.py')
limiter = Limiter(
    get_remote_address,
    app=app,
    storage_uri="memory://"
)

CORS(app)

try:
    with open("./days_since.json", "r") as f:
        days_since = json.loads(f.read())
except OSError:
    days_since = {}

@app.route("/")
def root():
  return "API server active!"

@app.get("/days-since")
def get_days_since():
    res = {key:f"{(date.today() - date.fromisoformat(value)).days} days since last broken {key} deployment" for key,value in days_since.items()}
    return jsonify(res)

@app.post("/days-since")
@limiter.limit("10/minute")
def set_days_since():
    """
    Antipates this format
    {"key":"password", "update": {"variantname":"2023-01-13"}}
    """
    try:
        if request.is_json:
            req_json = request.get_json()
            DAYS_SINCE_KEY = req_json["key"]
            if (DAYS_SINCE_KEY == app.config["DAYS_SINCE_KEY"]):
                print(req_json)
                update_obj = req_json["update"]
                for key,value in update_obj.items():
                    as_date = date.fromisoformat(value)
                        
                    days_since[key] = value
                print(days_since)
                with open("./days_since.json", "w") as f:
                    f.write(json.dumps(days_since))

                return json.dumps({'success':True}), 201, {'ContentType':'application/json'} 
            else:
                return json.dumps({"error", "Password incorrect"}), 401

    except ValueError:
        return json.dumps({"error": "Value error"}), 400
    except Exception as e:
        print(e)
        return json.dumps({"error": "Malformed"}), 400
    return json.dumps({"error": "Request must be JSON"}), 415

if __name__ == "__main__":
    app.run()