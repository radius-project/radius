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
# ------------------------------------------------------------

# parse an xml file and transform it into a junit xml file
# that can be used by the github actions junit reporter
# Path: .github/scripts/transform-test-results.py

import re
import sys
import xml.etree.ElementTree
import os


def main():
    if len(sys.argv) != 4:
        print(
            "Usage: transform-test-results.py <repository root> <input file> <output file>")
        sys.exit(1)

    repository_root = sys.argv[1]
    input_file = sys.argv[2]
    output_file = sys.argv[3]

    print(f"Processing {input_file}")
    
    # Check if input file exists
    if not os.path.exists(input_file):
        print(f"Input file {input_file} does not exist, skipping...")
        return
    
    # Check if input file is empty
    if os.path.getsize(input_file) == 0:
        print(f"Input file {input_file} is empty, skipping...")
        return
    
    pattern = re.compile(r"\tError Trace:\t(.*):(\d+)")
    try:
        et = xml.etree.ElementTree.parse(input_file)
    except xml.etree.ElementTree.ParseError as e:
        print(f"Error parsing XML file {input_file}: {e}")
        print("Skipping malformed XML file...")
        return
    except Exception as e:
        print(f"Unexpected error parsing {input_file}: {e}")
        print("Skipping file...")
        return
    for testcase in et.findall('./testsuite/testcase'):
        failure = testcase.find('./failure')
        if failure is None:
            continue

        # Extract file name by matching regex pattern in the text
        # it will look like \tError Trace:\tfilename:line
        match = pattern.search(failure.text)
        if match is None:
            continue

        file = match.group(1)
        line = match.group(2)

        # The filename will contain the fully-qualified path, and we need to turn that into
        # a relative path from the repository root
        if not file.startswith(repository_root):
            print(f"Could not find repository name in file path: {file}")
            continue

        file = file[len(repository_root) + 1:]

        testcase.attrib["file"] = file
        testcase.attrib["line"] = line
        failure.attrib["file"] = file
        failure.attrib["line"] = line

    # Write back to file
    try:
        print(f"Writing {output_file}")
        et.write(output_file)
    except Exception as e:
        print(f"Error writing output file {output_file}: {e}")
        print("Skipping file...")
        return


if __name__ == "__main__":
    main()
