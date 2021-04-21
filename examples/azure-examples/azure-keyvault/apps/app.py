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
client = SecretClient(vault_url=VAULT_URL, credential=managed_identity)
try:
    while True:
        # Create a secret
        print("\n.. Create Secret", flush=True)
        expires = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=365)
        secret_value = str(random.randint(1, 1000))
        secret_name = "mysecret-" + secret_value
        secret = client.set_secret(secret_name, secret_value, expires_on=expires)
        print("Secret with name '{0}' created with value '{1}'".format(secret.name, secret.value))

        # Retrieve the secret just created
        print("\n.. Get the Secret by name", flush=True)
        retrieved_secret = client.get_secret(secret_name)
        print("Secret with name '{0}' was found with value '{1}'.".format(retrieved_secret.name, retrieved_secret.value), flush=True)

        # Delete secret
        print("\n.. Deleting Secret...", flush=True)
        client.begin_delete_secret(retrieved_secret.name)
        print("Secret with name '{0}' was deleted.".format(retrieved_secret.name), flush=True)

        time.sleep(100)

except HttpResponseError as e:
    print("\nThis sample has caught an error. {0}".format(e.message), flush=True)