# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation and Dapr Contributors.
# Licensed under the MIT License.
# ------------------------------------------------------------

import json
import time

from dapr.clients import DaprClient

with DaprClient() as d:
    id=0
    while True:
        id+=1
        req_data = {
            'id': id,
            'message': 'hello world'
        }

        # Create a typed message with content type and body
        resp = d.publish_event(
            pubsub_name='pubsub',
            topic_name='TOPIC_A',
            data=json.dumps(req_data),
            data_content_type='application/json',
        )

        # Print the request
        print(req_data, flush=True)
        time.sleep(2)