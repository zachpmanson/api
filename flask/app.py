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
except:
    days_since = {}

@app.route("/")
def root():
    import urllib
    links = "<code><ul>"
    routes = ['%s' % rule for rule in app.url_map.iter_rules()]
    for route in app.url_map.iter_rules():
        links += f"<li><a href='{route}'>{route}</a>"
        links += f"<p style='margin-left:1rem'>{route.methods} {route.arguments}</p></li>"
    links+="</code>"
    return "API server active!" + links

@app.get("/days-since")
def get_days_since():
    res = {}
    for key,value in days_since.items():
        if value == "never":
            res[key] = f"{key} has never had a broken deployment"
        else:
            res[key] = f"{(date.today() - date.fromisoformat(value)).days} days since last broken {key} deployment"
    return jsonify(res), 200

@app.post("/days-since")
@limiter.limit("10/minute")
def set_days_since():
    """
    Antipates this format
    {"key":"password", "update": {"variantname":"2023-01-13"}}
    """
    try:
        if not request.is_json:
            return jsonify({"error": "Request must be JSON"}), 415
        
        req_body = request.get_json()
        
        if (req_body["key"] != app.config["DAYS_SINCE_KEY"]):
            return jsonify({"error": "Password incorrect"}), 401

        for key,value in req_body["update"].items():
            if value == "":
                del days_since[key]
            elif value == "never":
                days_since[key] = value
            else:
                days_since[key] = str(date.fromisoformat(value))

        with open("./days_since.json", "w") as f:
            f.write(json.dumps(days_since))

        return jsonify({'success':True}), 201


    except ValueError:
        return jsonify({"error": "Value error"}), 400
    except Exception as e:
        print(e)
        return jsonify({"error": f"Malformed - {e}"}), 400
    return jsonify({"error": "Unknown error"}), 400

if __name__ == "__main__":
    app.run()