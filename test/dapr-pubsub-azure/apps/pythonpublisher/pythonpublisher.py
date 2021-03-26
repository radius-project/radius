# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation and Dapr Contributors.
# Licensed under the MIT License.
# ------------------------------------------------------------

import json
import time
import os

from dapr.clients import DaprClient

pubsubName = os.environ.get('SB_PUBSUBNAME')
topic = os.environ.get('SB_TOPIC')

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
            pubsub_name=pubsubName,
            topic_name=topic,
            data=json.dumps(req_data),
            data_content_type='application/json',
        )

        # Print the request
        print(req_data, flush=True)
        time.sleep(2)