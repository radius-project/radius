#!/usr/bin/python3
# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#    
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# This script publishes a set of recipes for Terraform to a Kubernetes ConfigMap so that they can
# be used as a private registry.

import os
import sys
import tempfile
import contextlib
import shutil
import subprocess

if len(sys.argv) != 4:
    print("Usage: publish-test-terraform-recipes.py <recipe root> <namespace> <config map name>")
    sys.exit(1)
recipe_root = sys.argv[1]
namespace = sys.argv[2]
config_map_name = sys.argv[3]

# Write output to stdout all of the time, and the step summary if we're in Github Actions.
step_summary = os.getenv("GITHUB_STEP_SUMMARY")
with open(step_summary, "a") if step_summary else contextlib.suppress() as output:
    
    def log(message):
        print(message, flush=True)
        if step_summary:
            output.write(message + "\n")

    log("Publishing recipes from " + recipe_root + " to " + namespace + "/" + config_map_name)

    # Get the list of subfolders in the recipe root. Each one is a recipe.
    recipe_dirs = [f.path for f in os.scandir(recipe_root) if f.is_dir()]
    if len(recipe_dirs) == 0:
        print("No recipes found in " + recipe_root)
        sys.exit(1)

    # This is the data we'll use to create the ConfigMap. It's a map of recipe name to recipe to the
    # zip file we'll create.
    config_entries = {}

    # Use a temporary directory as scratch space. We need to zip each recipe, and want to clean
    # up after ourselves.
    with tempfile.TemporaryDirectory() as tmp_dir:
        for recipe_dir in recipe_dirs:
            log("Processing recipe: " + recipe_dir)

            # Make the zip.
            output_filename = shutil.make_archive(os.path.join(tmp_dir, os.path.basename(recipe_dir)), 'zip', recipe_dir)
            log("Created zip file: " + output_filename)

            # Add to config entries
            config_entries[os.path.basename(recipe_dir)] = output_filename

        # Delete the configmap if it already exists
        args = ["kubectl", "delete", "configmap", config_map_name, "--namespace", namespace, "--ignore-not-found=true"]
        process = subprocess.run(args)
        process.check_returncode()

        # Create the configmap
        args = ["kubectl", "create", "configmap", config_map_name, "--namespace", namespace]
        for recipe_name, zip_file in config_entries.items():
            args.append("--from-file=" + recipe_name + ".zip=" + zip_file)

        process = subprocess.run(args)
        process.check_returncode()
        log("Created configmap: " + namespace + "/" + config_map_name)