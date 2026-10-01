# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
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

{
  description = "Radius, a cloud-native application platform: the rad CLI";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs =
    { self, nixpkgs }:
    let
      inherit (nixpkgs) lib;

      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = lib.genAttrs systems;

      # Read a `NAME ?= value` default from one of the Makefile includes, so
      # the flake stamps the same values as `make build-rad`.
      makeDefault =
        file: name:
        let
          match = builtins.match ".*\n${name}[[:space:]]*\\?=[[:space:]]*([^[:space:]]+)\n.*" (
            builtins.readFile file
          );
        in
        if match == null then throw "flake.nix: ${name} not found in ${toString file}" else builtins.head match;

      relChannel = makeDefault ./build/version.mk "REL_CHANNEL";
      relVersion = makeDefault ./build/version.mk "REL_VERSION";
      chartVersion = makeDefault ./build/version.mk "CHART_VERSION";
      terraformVersion = makeDefault ./build/tools.generated.mk "TERRAFORM_VERSION";

      # build/version.mk sets GIT_COMMIT from `git rev-list -1 HEAD` and
      # GIT_VERSION from `git describe --always --abbrev=7 --dirty --tags`.
      # Flakes do not expose tags, so the version is the abbreviated commit,
      # with a "-dirty" suffix for uncommitted changes, as `git describe`
      # prints when no tag is reachable. Sources without git metadata fall
      # back to the defaults in pkg/version/version.go.
      gitCommit = self.rev or (lib.removeSuffix "-dirty" (self.dirtyRev or "unknown"));
      gitVersion = self.shortRev or self.dirtyShortRev or "edge";

      # nixpkgs-style version for an unreleased build: 0-unstable-YYYY-MM-DD.
      lastModified = self.lastModifiedDate or "19700101000000";
      buildDate = "${builtins.substring 0 4 lastModified}-${builtins.substring 4 2 lastModified}-${builtins.substring 6 2 lastModified}";
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          basePackage = "github.com/radius-project/radius";
        in
        {
          # Use the Go release that go.mod requires.
          rad = pkgs.buildGo127Module {
            pname = "rad";
            version = "0-unstable-${buildDate}";

            src = self;

            # Update this whenever go.mod or go.sum changes; `nix build`
            # prints the expected value on a mismatch.
            vendorHash = "sha256-fxDRrsmA0XIogy5XseISUf6la07SqQ/3DbHU8xdg+Oc=";

            subPackages = [ "cmd/rad" ];

            env.CGO_ENABLED = 0;

            # Same linker flags as LDFLAGS in build/build.mk.
            ldflags = [
              "-s"
              "-w"
              "-X ${basePackage}/pkg/version.channel=${relChannel}"
              "-X ${basePackage}/pkg/version.release=${relVersion}"
              "-X ${basePackage}/pkg/version.commit=${gitCommit}"
              "-X ${basePackage}/pkg/version.version=${gitVersion}"
              "-X ${basePackage}/pkg/version.chartVersion=${chartVersion}"
              "-X ${basePackage}/pkg/recipes/terraform.terraformVersion=${terraformVersion}"
            ];

            meta = {
              description = "Command-line interface for Radius, a cloud-native application platform";
              homepage = "https://radapp.io";
              license = lib.licenses.asl20;
              mainProgram = "rad";
            };
          };

          default = self.packages.${system}.rad;
        }
      );

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = lib.getExe self.packages.${system}.rad;
          meta.description = "Run the Radius rad CLI";
        };
      });
    };
}
