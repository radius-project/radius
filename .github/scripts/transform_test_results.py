# parse an xml file and transform it into a junit xml file
# that can be used by the github actions junit reporter
# Path: .github/scripts/transform-test-results.py

import re
import sys
import xml.etree.ElementTree

repository = "project-radius/radius"

def main():
    if len(sys.argv) != 4:
        print("Usage: transform-test-results.py <repository root> <input file> <output file>")
        sys.exit(1)

    repository_root = sys.argv[1]
    input_file = sys.argv[2]
    output_file = sys.argv[3]

    pattern = re.compile(r"\tError Trace:\t(.*):(\d+)")
    et = xml.etree.ElementTree.parse(input_file)
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
    et.write(output_file)

if __name__ == "__main__":
    main()
