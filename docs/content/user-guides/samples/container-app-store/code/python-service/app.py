import os
import logging
import flask
from flask import request, jsonify
from flask import json
from flask_cors import CORS
from dapr.clients import DaprClient

logging.basicConfig(level=logging.INFO)

app = flask.Flask(__name__)
CORS(app)

@app.route('/order', methods=['GET'])
def getOrder():
    app.logger.info('order service called')
    with DaprClient() as d:
        d.wait(5)
        try:
            id = request.args.get('id')
            if id:
                # Get the order status from Cosmos DB via Dapr
                state = d.get_state(store_name='orders', key=id)
                if state.data:
                    resp = jsonify(json.loads(state.data))
                else:
                    resp = jsonify('no order with that id found')
                resp.status_code = 200
                return resp
            else:
                resp = jsonify('Order "id" not found in query string')
                resp.status_code = 500
                return resp
        except Exception as e:
            app.logger.info(e)
            return str(e)
        finally:
            app.logger.info('completed order call')

@app.route('/order', methods=['POST'])
def createOrder():
    app.logger.info('create order called')
    with DaprClient() as d:
        d.wait(5)
        try:
            # Get ID from the request body
            id = request.json['id']
            if id:
                # Save the order to Cosmos DB via Dapr
                d.save_state(store_name='orders', key=id, value=json.dumps(request.json))
                resp = jsonify(request.json)
                resp.status_code = 200
                return resp
            else:
                resp = jsonify('Order "id" not found in query string')
                resp.status_code = 500
                return resp
        except Exception as e:
            app.logger.info(e)
            return str(e)
        finally:
            app.logger.info('created order')

app.run(host='0.0.0.0', port=os.getenv('PORT', '5000'))