# ------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------
import datetime
import os
import time
import random
from azure.keyvault.secrets import SecretClient
from azure.identity import ManagedIdentityCredential
from azure.core.exceptions import HttpResponseError

# Test Application that tries to access Keyvault
print("Getting vault url", flush=True)
VAULT_URL = os.environ["KV_URI"]
print("Vault url: {0}".format(VAULT_URL), flush=True)
managed_identity = ManagedIdentityCredential()
try:
    while True:
        # List secrets
        client = SecretClient(vault_url=VAULT_URL, credential=managed_identity)
        print("\n.. List Secrets", flush=True)
        secret_properties = client.list_properties_of_secrets()
        for secret_property in secret_properties:
            # the list doesn't include values or versions of the secrets
            print(secret_property.name, flush=True)

        time.sleep(2000)

except HttpResponseError as e:
    print("\nThis sample has caught an error. {0}".format(e.message), flush=True)